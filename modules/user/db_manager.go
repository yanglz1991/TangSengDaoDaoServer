package user

import (
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/gocraft/dbr/v2"
)

type managerDB struct {
	session *dbr.Session
	ctx     *config.Context
}

// newManagerDB
func newManagerDB(ctx *config.Context) *managerDB {
	return &managerDB{
		ctx:     ctx,
		session: ctx.DB(),
	}
}

// 通过账号和密码查询用户信息
func (m *managerDB) queryUserInfoWithNameAndPwd(username string) (*managerLoginModel, error) {
	var model *managerLoginModel
	_, err := m.session.Select("*").From("user").Where("username=?", username).Load(&model)
	return model, err
}

// 获取用户列表
func (m *managerDB) queryUserListWithPage(pageSize, page uint64, onelineStatus int) ([]*managerUserModel, error) {
	// var users []*managerUserModel
	// _, err := m.session.Select("*").From("user").Offset((page-1)*pageSize).Limit(pageSize).OrderDir("created_at", false).Load(&users)
	// return users, err

	var users []*managerUserModel
	selectStm := m.session.Select("user.uid,user.name,user.username,user.status,user.phone,user.short_no,user.sex,user.is_destroy,user.created_at,user.gitee_uid,user.github_uid,user.wx_openid,max(user_online.online) online").From("user").LeftJoin("user_online", "user.uid=user_online.uid")
	if onelineStatus != -1 {
		selectStm = selectStm.Where("user_online.online=?", onelineStatus)
	}
	selectStm = selectStm.GroupBy("user.uid,user.name,user.username,user.status,user.phone,user.short_no,user.sex,user.is_destroy,user.created_at,user.gitee_uid,user.github_uid,user.wx_openid")

	// select  from user left join user_online on user.uid=user_online.uid where user_online.online=1  group by user.uid,user.name,user.status,user.phone,user.short_no,user.sex,user.is_destroy,user.created_at  limit 100
	_, err := selectStm.Offset((page-1)*pageSize).Limit(pageSize).OrderDir("user.created_at", false).Load(&users)
	return users, err
}

// 模糊查询用户列表
// onelineStatus 在线状态 -1 为所有 0. 离线 1. 在线
// 关键字匹配范围：user.name / user.uid / user.phone / user.short_no
// 以及 login_log.login_ip(IP) 与 device.device_name / device.device_model(设备)
func (m *managerDB) queryUserListWithPageAndKeyword(keyword string, onelineStatus int, pageSize, page uint64) ([]*managerUserModel, error) {
	var users []*managerUserModel
	like := "%" + keyword + "%"
	selectStm := m.session.Select("user.uid,user.name,user.username,user.status,user.phone,user.short_no,user.sex,user.is_destroy,user.created_at,user.gitee_uid,user.github_uid,user.wx_openid,max(user_online.online) online").From("user").LeftJoin("user_online", "user.uid=user_online.uid").Where("user.name like ? or user.uid like ? or user.phone like ? or user.short_no like ? or user.uid in (select uid from login_log where login_ip like ?) or user.uid in (select uid from device where device_name like ? or device_model like ?)", like, like, like, like, like, like, like)
	if onelineStatus != -1 {
		selectStm = selectStm.Where("user_online.online=?", onelineStatus)
	}
	selectStm = selectStm.GroupBy("user.uid,user.name,user.username,user.status,user.phone,user.short_no,user.sex,user.is_destroy,user.created_at,user.gitee_uid,user.github_uid,user.wx_openid")

	// select  from user left join user_online on user.uid=user_online.uid where user_online.online=1  group by user.uid,user.name,user.status,user.phone,user.short_no,user.sex,user.is_destroy,user.created_at  limit 100
	_, err := selectStm.Offset((page-1)*pageSize).Limit(pageSize).OrderDir("user.created_at", false).Load(&users)
	return users, err
}

// 模糊查询用户数量
// 关键字匹配范围与 queryUserListWithPageAndKeyword 保持一致
func (m *managerDB) queryUserCountWithKeyWord(keyword string) (int64, error) {
	var count int64
	like := "%" + keyword + "%"
	_, err := m.session.Select("count(*)").From("user").Where("name like ? or uid like ? or phone like ? or short_no like ? or uid in (select uid from login_log where login_ip like ?) or uid in (select uid from device where device_name like ? or device_model like ?)", like, like, like, like, like, like, like).Load(&count)
	return count, err
}

// queryUserBlacklist 查询某个用户的黑名单
func (m *managerDB) queryUserBlacklists(uid string) ([]*managerUserBlacklistModel, error) {
	var users []*managerUserBlacklistModel
	_, err := m.session.Select("user_setting.*,`user`.name,`user`.uid").From("user_setting").LeftJoin("user", "user_setting.to_uid=user.uid").Where("user_setting.uid=? and user_setting.blacklist=1", uid).Load(&users)
	return users, err
}

// 通过status查询用户列表
func (m *managerDB) queryUserListWithStatus(status int, pageSize, page uint64) ([]*managerUserModel, error) {
	var users []*managerUserModel
	_, err := m.session.Select("*").From("user").Where("status=?", status).Offset((page-1)*pageSize).Limit(pageSize).OrderDir("updated_at", false).Load(&users)
	return users, err
}

// 通过status查询用户数量
func (m *managerDB) queryUserCountWithStatus(status int) (int64, error) {
	var count int64
	_, err := m.session.Select("count(*)").From("user").Where("status=?", status).Load(&count)
	return count, err
}

// 查询 IP 聚合列表（按 (login_ip, uid) 去重，取最近登录时间倒序）
// keyword 模糊匹配 IP / 用户 uid / 用户 name
func (m *managerDB) queryIPAggList(keyword string, pageSize, page uint64) ([]*managerIPAggModel, error) {
	var list []*managerIPAggModel
	stmt := m.session.SelectBySql(`
		select t.login_ip, t.uid, t.last_login_at, u.name, u.username, u.status
		from (
			select login_ip, uid, max(created_at) as last_login_at
			from login_log
			group by login_ip, uid
		) t
		left join user u on u.uid = t.uid
		`+ipAggKeywordWhere(keyword)+`
		order by t.last_login_at desc
		limit ? offset ?
	`, ipAggKeywordArgs(keyword, pageSize, (page-1)*pageSize)...)
	_, err := stmt.Load(&list)
	return list, err
}

func (m *managerDB) queryIPAggCount(keyword string) (int64, error) {
	var count int64
	_, err := m.session.SelectBySql(`
		select count(*) from (
			select login_ip, uid from login_log group by login_ip, uid
		) t left join user u on u.uid = t.uid
		`+ipAggKeywordWhere(keyword), ipAggKeywordArgsForCount(keyword)...).Load(&count)
	return count, err
}

func ipAggKeywordWhere(keyword string) string {
	if keyword == "" {
		return ""
	}
	return " where (t.login_ip like ? or t.uid like ? or u.name like ?) "
}

func ipAggKeywordArgs(keyword string, limit, offset uint64) []interface{} {
	if keyword == "" {
		return []interface{}{limit, offset}
	}
	like := "%" + keyword + "%"
	return []interface{}{like, like, like, limit, offset}
}

func ipAggKeywordArgsForCount(keyword string) []interface{} {
	if keyword == "" {
		return nil
	}
	like := "%" + keyword + "%"
	return []interface{}{like, like, like}
}

// 查询设备列表（每个 (device_id, uid) 一行，按最后登录时间倒序）
// keyword 模糊匹配 device_id / device_name / device_model / 用户 uid / 用户 name
func (m *managerDB) queryDeviceAggList(keyword string, pageSize, page uint64) ([]*managerDeviceAggModel, error) {
	var list []*managerDeviceAggModel
	_, err := m.session.SelectBySql(`
		select d.id, d.device_id, d.uid, d.device_name, d.device_model, d.last_login, u.name, u.username, u.status
		from device d
		left join user u on u.uid = d.uid
		`+deviceAggKeywordWhere(keyword)+`
		order by d.last_login desc
		limit ? offset ?
	`, deviceAggKeywordArgs(keyword, pageSize, (page-1)*pageSize)...).Load(&list)
	return list, err
}

func (m *managerDB) queryDeviceAggCount(keyword string) (int64, error) {
	var count int64
	_, err := m.session.SelectBySql(`
		select count(*) from device d left join user u on u.uid = d.uid
		`+deviceAggKeywordWhere(keyword), deviceAggKeywordArgsForCount(keyword)...).Load(&count)
	return count, err
}

func deviceAggKeywordWhere(keyword string) string {
	if keyword == "" {
		return ""
	}
	return " where (d.device_id like ? or d.device_name like ? or d.device_model like ? or d.uid like ? or u.name like ?) "
}

func deviceAggKeywordArgs(keyword string, limit, offset uint64) []interface{} {
	if keyword == "" {
		return []interface{}{limit, offset}
	}
	like := "%" + keyword + "%"
	return []interface{}{like, like, like, like, like, limit, offset}
}

func deviceAggKeywordArgsForCount(keyword string) []interface{} {
	if keyword == "" {
		return nil
	}
	like := "%" + keyword + "%"
	return []interface{}{like, like, like, like, like}
}

// queryOnlineUIDsByLastLoginIP 查询「当前在线 且 最近一次登录 IP == ip」的 uid 集合
// 用于 IP 黑名单封禁时，推送 forceLogout CMD 给可能正在使用此 IP 的在线用户
// 近似实现：实际客户端切网后 publicIP 变化但 IM 长连仍存活时会漏踢；不会误踢他人
func (m *managerDB) queryOnlineUIDsByLastLoginIP(ip string) ([]string, error) {
	if ip == "" {
		return nil, nil
	}
	var uids []string
	_, err := m.session.SelectBySql(`
		select uo.uid
		from user_online uo
		join (
			select uid, login_ip
			from login_log
			where (uid, created_at) in (select uid, max(created_at) from login_log group by uid)
		) ll on ll.uid = uo.uid
		where uo.online = 1 and ll.login_ip = ?
		group by uo.uid
	`, ip).Load(&uids)
	return uids, err
}

// queryUIDsByDeviceID 查询此 device_id 关联的所有 uid
func (m *managerDB) queryUIDsByDeviceID(deviceID string) ([]string, error) {
	if deviceID == "" {
		return nil, nil
	}
	var uids []string
	_, err := m.session.Select("uid").From("device").Where("device_id=?", deviceID).GroupBy("uid").Load(&uids)
	return uids, err
}

func (m *managerDB) queryUserOnline(uid string) ([]*userOnline, error) {
	var list []*userOnline
	_, err := m.session.Select("*").From("user_online").Where("uid=?", uid).Load(&list)
	return list, err
}

func (m *managerDB) queryUserWithNameAndRole(username string, role string) (*managerUserModel, error) {
	var user *managerUserModel
	_, err := m.session.Select("*").From("user").Where("username=? and role=?", username, role).Load(&user)
	return user, err
}

func (m *managerDB) queryUsersWithRole(role string) ([]*managerUserModel, error) {
	var list []*managerUserModel
	_, err := m.session.Select("*").From("user").Where("role=?", role).Load(&list)
	return list, err
}
func (m *managerDB) deleteUserWithUIDAndRole(uid, role string) error {
	_, err := m.session.DeleteFrom("user").Where("uid=? and role=?", uid, role).Exec()
	return err
}

type managerLoginModel struct {
	Username string
	UID      string
	Name     string
	Password string
	Role     string
}

type managerUserModel struct {
	Username  string
	Name      string
	UID       string
	Status    int
	Phone     string
	ShortNo   string
	WXOpenid  string // 微信openid
	GiteeUID  string // gitee uid
	GithubUID string // github uid
	Sex       int
	IsDestroy int
	db.BaseModel
}

type managerUserBlacklistModel struct {
	Name string
	UID  string
	db.BaseModel
}

// managerIPAggModel IP 聚合视图：(login_ip, uid) 唯一
type managerIPAggModel struct {
	LoginIP     string       `db:"login_ip"`
	UID         string       `db:"uid"`
	LastLoginAt dbr.NullTime `db:"last_login_at"`
	Name        string       `db:"name"`
	Username    string       `db:"username"`
	Status      int          `db:"status"`
}

// managerDeviceAggModel 设备列表视图：(device_id, uid) 唯一
type managerDeviceAggModel struct {
	Id          int64  `db:"id"`
	DeviceID    string `db:"device_id"`
	UID         string `db:"uid"`
	DeviceName  string `db:"device_name"`
	DeviceModel string `db:"device_model"`
	LastLogin   int64  `db:"last_login"`
	Name        string `db:"name"`
	Username    string `db:"username"`
	Status      int    `db:"status"`
}

type userOnline struct {
	UID         string
	DeviceFlag  uint8 // 设备标记 0. APP 1.web
	LastOnline  int   // 最后一次在线时间
	LastOffline int   // 最后一次离线时间
	Online      int
	Version     int64 // 数据版本
	db.BaseModel
}
