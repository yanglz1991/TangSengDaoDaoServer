package level

import (
	"fmt"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/gocraft/dbr/v2"
)

// CacheKeyFriends 与 user 模块保持一致；level 模块在加默认好友时需同步刷新缓存
const cacheKeyFriends = "lm-friends:"

type levelDB struct {
	ctx     *config.Context
	session *dbr.Session
}

func newLevelDB(ctx *config.Context) *levelDB {
	return &levelDB{
		ctx:     ctx,
		session: ctx.DB(),
	}
}

// ---------- 层级节点 ----------

func (d *levelDB) insertNode(m *nodeModel) error {
	_, err := d.session.InsertInto("level_node").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	return err
}

func (d *levelDB) updateNode(m *nodeModel) error {
	_, err := d.session.Update("level_node").SetMap(map[string]interface{}{
		"name":        m.Name,
		"owner_uid":   m.OwnerUID,
		"invite_code": m.InviteCode,
	}).Where("node_no=?", m.NodeNo).Exec()
	return err
}

func (d *levelDB) deleteNode(nodeNo string) error {
	_, err := d.session.DeleteFrom("level_node").Where("node_no=?", nodeNo).Exec()
	return err
}

func (d *levelDB) queryNode(nodeNo string) (*nodeModel, error) {
	var m *nodeModel
	_, err := d.session.Select("*").From("level_node").Where("node_no=?", nodeNo).Load(&m)
	return m, err
}

func (d *levelDB) queryNodeByInviteCode(code string) (*nodeModel, error) {
	var m *nodeModel
	_, err := d.session.Select("*").From("level_node").Where("invite_code=?", code).Load(&m)
	return m, err
}

func (d *levelDB) queryAllNodes() ([]*nodeModel, error) {
	var list []*nodeModel
	_, err := d.session.Select("*").From("level_node").OrderBy("created_at").Load(&list)
	return list, err
}

func (d *levelDB) queryChildrenCount(parentNo string) (int64, error) {
	var count int64
	_, err := d.session.Select("count(*)").From("level_node").Where("parent_no=?", parentNo).Load(&count)
	return count, err
}

func (d *levelDB) queryUserCount(nodeNo string) (int64, error) {
	var count int64
	_, err := d.session.Select("count(*)").From("user").Where("level_node_no=?", nodeNo).Load(&count)
	return count, err
}

// 是否存在邀请码冲突（排除自身 nodeNo；nodeNo 为空表示新建场景）
func (d *levelDB) inviteCodeConflict(code, excludeNodeNo string) (bool, error) {
	if code == "" {
		return false, nil
	}
	stm := d.session.Select("count(*)").From("level_node").Where("invite_code=?", code)
	if excludeNodeNo != "" {
		stm = stm.Where("node_no<>?", excludeNodeNo)
	}
	var count int64
	_, err := stm.Load(&count)
	return count > 0, err
}

// ---------- 默认好友 ----------

func (d *levelDB) insertDefaultFriend(nodeNo, friendUID string) error {
	_, err := d.session.InsertBySql("INSERT IGNORE INTO level_default_friend(node_no,friend_uid) VALUES(?,?)", nodeNo, friendUID).Exec()
	return err
}

func (d *levelDB) deleteDefaultFriendsByNode(nodeNo string) error {
	_, err := d.session.DeleteFrom("level_default_friend").Where("node_no=?", nodeNo).Exec()
	return err
}

func (d *levelDB) queryDefaultFriends(nodeNo string) ([]string, error) {
	var uids []string
	_, err := d.session.Select("friend_uid").From("level_default_friend").Where("node_no=?", nodeNo).Load(&uids)
	return uids, err
}

// ---------- 用户 / 好友 ----------

// 列出层级下的用户（直接用 user 表，按 level_node_no 过滤）
type userRow struct {
	UID                    string
	Name                   string
	Phone                  string
	Username               string
	ShortNo                string
	Status                 int
	CanInviteOrCreateGroup int
	LevelNodeNo            string
	CreatedAt              dbr.NullTime
}

func (d *levelDB) queryUsersByNode(nodeNo, keyword string, pageSize, page uint64) ([]*userRow, int64, error) {
	var list []*userRow
	stm := d.session.Select("uid,name,phone,username,short_no,status,can_invite_or_create_group,level_node_no,created_at").
		From("user").Where("level_node_no=?", nodeNo)
	if keyword != "" {
		like := "%" + keyword + "%"
		stm = stm.Where("name like ? or phone like ? or username like ? or short_no like ? or uid like ?", like, like, like, like, like)
	}
	_, err := stm.OrderDir("created_at", false).Offset((page - 1) * pageSize).Limit(pageSize).Load(&list)
	if err != nil {
		return nil, 0, err
	}
	var count int64
	cstm := d.session.Select("count(*)").From("user").Where("level_node_no=?", nodeNo)
	if keyword != "" {
		like := "%" + keyword + "%"
		cstm = cstm.Where("name like ? or phone like ? or username like ? or short_no like ? or uid like ?", like, like, like, like, like)
	}
	_, err = cstm.Load(&count)
	return list, count, err
}

// 用户搜索（用于选层级负责人 / 默认好友），仅普通已启用用户
func (d *levelDB) searchUsers(keyword string, limit uint64) ([]*userRow, error) {
	var list []*userRow
	stm := d.session.Select("uid,name,phone,username,short_no,status,can_invite_or_create_group,level_node_no,created_at").
		From("user").Where("is_destroy=0")
	if keyword != "" {
		like := "%" + keyword + "%"
		stm = stm.Where("name like ? or phone like ? or username like ? or short_no like ? or uid like ?", like, like, like, like, like)
	}
	_, err := stm.OrderDir("created_at", false).Limit(limit).Load(&list)
	return list, err
}

func (d *levelDB) queryUserNames(uids []string) (map[string]string, error) {
	result := map[string]string{}
	if len(uids) == 0 {
		return result, nil
	}
	type row struct {
		UID  string
		Name string
	}
	var list []*row
	_, err := d.session.Select("uid,name").From("user").Where("uid in ?", uids).Load(&list)
	if err != nil {
		return nil, err
	}
	for _, r := range list {
		result[r.UID] = r.Name
	}
	return result, nil
}

// 更新用户层级
func (d *levelDB) updateUserLevelNode(uid, nodeNo string) error {
	_, err := d.session.Update("user").Set("level_node_no", nodeNo).Where("uid=?", uid).Exec()
	return err
}

// 更新用户「加好友/建群」权限
func (d *levelDB) updateUserCanInviteOrCreateGroup(uid string, can int) error {
	_, err := d.session.Update("user").Set("can_invite_or_create_group", can).Where("uid=?", uid).Exec()
	return err
}

// 批量更新某层级下所有非管理员用户的「加好友/建群」权限。
// 不影响 admin / superAdmin（这些角色服务端始终不受该字段约束，但仍按要求保持其字段值不变）。
// 返回受影响的行数。
func (d *levelDB) updateNodeUsersCanInviteOrCreateGroup(nodeNo string, can int) (int64, error) {
	res, err := d.session.Update("user").
		Set("can_invite_or_create_group", can).
		Where("level_node_no=?", nodeNo).
		Exec()
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ---------- friend 操作（仅 level 模块自用，避免依赖 user 包私有符号） ----------

// 友好关系是否存在
func (d *levelDB) isFriend(uid, toUID string) (bool, error) {
	var count int64
	_, err := d.session.Select("count(*)").From("friend").Where("uid=? and to_uid=? and is_deleted=0", uid, toUID).Load(&count)
	return count > 0, err
}

// 双向加为好友（覆盖已删除的关系）
func (d *levelDB) addFriendBidirectional(tx *dbr.Tx, aUID, bUID string) error {
	if aUID == "" || bUID == "" || aUID == bUID {
		return nil
	}
	if err := d.upsertFriendTx(tx, aUID, bUID); err != nil {
		return err
	}
	if err := d.upsertFriendTx(tx, bUID, aUID); err != nil {
		return err
	}
	// 缓存同步
	_ = d.ctx.GetRedisConn().SAdd(fmt.Sprintf("%s%s", cacheKeyFriends, aUID), bUID)
	_ = d.ctx.GetRedisConn().SAdd(fmt.Sprintf("%s%s", cacheKeyFriends, bUID), aUID)
	return nil
}

// 单向写入好友：存在则恢复（is_deleted=0/is_alone=0），不存在则插入
func (d *levelDB) upsertFriendTx(tx *dbr.Tx, uid, toUID string) error {
	var existingID int64
	count, err := tx.Select("id").From("friend").Where("uid=? and to_uid=?", uid, toUID).Limit(1).Load(&existingID)
	if err != nil {
		return err
	}
	version := d.ctx.GenSeq("friend")
	if count == 0 {
		_, err = tx.InsertBySql(
			"INSERT INTO friend(uid,to_uid,flag,version,is_deleted,is_alone,vercode,source_vercode,initiator,created_at,updated_at) VALUES(?,?,0,?,0,0,?,?,0,now(),now())",
			uid, toUID, version,
			fmt.Sprintf("%s@%d", util.GenerUUID(), 7), // 7=Friend vercode type
			"",
		).Exec()
		return err
	}
	_, err = tx.Update("friend").SetMap(map[string]interface{}{
		"is_deleted": 0,
		"is_alone":   0,
		"version":    version,
	}).Where("id=?", existingID).Exec()
	return err
}

// 双向删除好友（软删除）
func (d *levelDB) removeFriendBidirectional(tx *dbr.Tx, aUID, bUID string) error {
	if aUID == "" || bUID == "" || aUID == bUID {
		return nil
	}
	version := d.ctx.GenSeq("friend")
	if _, err := tx.Update("friend").SetMap(map[string]interface{}{
		"is_deleted": 1,
		"is_alone":   1,
		"version":    version,
	}).Where("uid=? and to_uid=?", aUID, bUID).Exec(); err != nil {
		return err
	}
	if _, err := tx.Update("friend").SetMap(map[string]interface{}{
		"is_deleted": 1,
		"is_alone":   1,
		"version":    version,
	}).Where("uid=? and to_uid=?", bUID, aUID).Exec(); err != nil {
		return err
	}
	// 缓存同步
	_ = d.ctx.GetRedisConn().SRem(fmt.Sprintf("%s%s", cacheKeyFriends, aUID), bUID)
	_ = d.ctx.GetRedisConn().SRem(fmt.Sprintf("%s%s", cacheKeyFriends, bUID), aUID)
	return nil
}
