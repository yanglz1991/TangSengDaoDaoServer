package level

import (
	"strconv"
	"strings"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/log"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

// API 层级管理后台 API
type API struct {
	ctx *config.Context
	log.Log
	db *levelDB
}

// NewAPI 创建
func NewAPI(ctx *config.Context) *API {
	return &API{
		ctx: ctx,
		Log: log.NewTLog("level"),
		db:  newLevelDB(ctx),
	}
}

// Route 路由
func (a *API) Route(r *wkhttp.WKHttp) {
	auth := r.Group("/v1/manager/level", a.ctx.AuthMiddleware(r))
	{
		auth.GET("/tree", a.tree)                                     // 层级树
		auth.GET("/node/:node_no", a.nodeDetail)                      // 层级详情
		auth.POST("/node", a.createNode)                              // 创建层级
		auth.PUT("/node/:node_no", a.updateNode)                      // 修改层级
		auth.DELETE("/node/:node_no", a.deleteNode)                   // 删除层级
		auth.GET("/node/:node_no/users", a.listUsers)                 // 层级下用户列表
		auth.GET("/users/search", a.searchUsersAPI)                   // 搜索用户（用于负责人/默认好友选择）
		auth.PUT("/user/permission", a.updateUserPermission)          // 切换用户加人/建群权限
		auth.PUT("/node/:node_no/permission", a.updateNodePermission) // 整个层级一键切换加人/建群权限
		auth.PUT("/user/move", a.moveUser)                            // 移动用户到指定层级（或添加到层级）
		auth.PUT("/user/remove", a.removeUser)                        // 把用户移出当前层级（同时解除该层级默认好友）
	}
}

// ----------------- handlers -----------------

// 层级树（扁平返回，前端拼成树）
func (a *API) tree(c *wkhttp.Context) {
	if err := c.CheckLoginRole(); err != nil {
		c.ResponseError(err)
		return
	}
	nodes, err := a.db.queryAllNodes()
	if err != nil {
		a.Error("查询层级失败", zap.Error(err))
		c.ResponseError(errors.New("查询层级失败"))
		return
	}
	uids := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if n.OwnerUID != "" {
			uids = append(uids, n.OwnerUID)
		}
	}
	nameMap, _ := a.db.queryUserNames(uids)
	list := make([]*nodeDetail, 0, len(nodes))
	for _, n := range nodes {
		count, _ := a.db.queryUserCount(n.NodeNo)
		list = append(list, &nodeDetail{
			NodeNo:     n.NodeNo,
			ParentNo:   n.ParentNo,
			Name:       n.Name,
			OwnerUID:   n.OwnerUID,
			OwnerName:  nameMap[n.OwnerUID],
			InviteCode: n.InviteCode,
			UserCount:  count,
			CreatedAt:  n.CreatedAt.String(),
			UpdatedAt:  n.UpdatedAt.String(),
		})
	}
	c.Response(list)
}

// 层级详情（含默认好友列表）
func (a *API) nodeDetail(c *wkhttp.Context) {
	if err := c.CheckLoginRole(); err != nil {
		c.ResponseError(err)
		return
	}
	nodeNo := c.Param("node_no")
	n, err := a.db.queryNode(nodeNo)
	if err != nil {
		a.Error("查询层级失败", zap.Error(err))
		c.ResponseError(errors.New("查询层级失败"))
		return
	}
	if n == nil {
		c.ResponseError(errors.New("层级不存在"))
		return
	}
	friends, err := a.db.queryDefaultFriends(nodeNo)
	if err != nil {
		c.ResponseError(errors.New("查询默认好友失败"))
		return
	}
	uids := append([]string{}, friends...)
	if n.OwnerUID != "" {
		uids = append(uids, n.OwnerUID)
	}
	nameMap, _ := a.db.queryUserNames(uids)
	count, _ := a.db.queryUserCount(nodeNo)
	type friendItem struct {
		UID  string `json:"uid"`
		Name string `json:"name"`
	}
	defaultFriends := make([]friendItem, 0, len(friends))
	for _, uid := range friends {
		defaultFriends = append(defaultFriends, friendItem{UID: uid, Name: nameMap[uid]})
	}
	c.Response(map[string]interface{}{
		"node_no":         n.NodeNo,
		"parent_no":       n.ParentNo,
		"name":            n.Name,
		"owner_uid":       n.OwnerUID,
		"owner_name":      nameMap[n.OwnerUID],
		"invite_code":     n.InviteCode,
		"user_count":      count,
		"default_friends": defaultFriends,
		"created_at":      n.CreatedAt.String(),
		"updated_at":      n.UpdatedAt.String(),
	})
}

type nodeReq struct {
	ParentNo          string   `json:"parent_no"`
	Name              string   `json:"name"`
	OwnerUID          string   `json:"owner_uid"`
	InviteCode        string   `json:"invite_code"`
	DefaultFriendUIDs []string `json:"default_friend_uids"`
}

func (r *nodeReq) check() error {
	r.Name = strings.TrimSpace(r.Name)
	r.InviteCode = strings.TrimSpace(r.InviteCode)
	r.OwnerUID = strings.TrimSpace(r.OwnerUID)
	r.ParentNo = strings.TrimSpace(r.ParentNo)
	if r.Name == "" {
		return errors.New("层级名称不能为空")
	}
	if r.OwnerUID == "" {
		return errors.New("请选择层级负责人")
	}
	if r.InviteCode == "" {
		return errors.New("邀请码不能为空")
	}
	if len(r.InviteCode) < 4 || len(r.InviteCode) > 32 {
		return errors.New("邀请码长度需在 4-32 之间")
	}
	return nil
}

// 创建层级
func (a *API) createNode(c *wkhttp.Context) {
	if err := c.CheckLoginRole(); err != nil {
		c.ResponseError(err)
		return
	}
	var req nodeReq
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误"))
		return
	}
	if err := req.check(); err != nil {
		c.ResponseError(err)
		return
	}
	// 父层级存在性校验
	if req.ParentNo != "" {
		parent, err := a.db.queryNode(req.ParentNo)
		if err != nil || parent == nil {
			c.ResponseError(errors.New("父层级不存在"))
			return
		}
	}
	// 邀请码冲突
	conflict, err := a.db.inviteCodeConflict(req.InviteCode, "")
	if err != nil {
		a.Error("校验邀请码冲突失败", zap.Error(err))
		c.ResponseError(errors.New("校验邀请码冲突失败"))
		return
	}
	if conflict {
		c.ResponseError(errors.New("邀请码已被占用"))
		return
	}
	nodeNo := util.GenerUUID()
	n := &nodeModel{
		NodeNo:     nodeNo,
		ParentNo:   req.ParentNo,
		Name:       req.Name,
		OwnerUID:   req.OwnerUID,
		InviteCode: req.InviteCode,
	}
	if err := a.db.insertNode(n); err != nil {
		a.Error("创建层级失败", zap.Error(err))
		c.ResponseError(errors.New("创建层级失败"))
		return
	}
	for _, uid := range req.DefaultFriendUIDs {
		uid = strings.TrimSpace(uid)
		if uid == "" {
			continue
		}
		if err := a.db.insertDefaultFriend(nodeNo, uid); err != nil {
			a.Warn("写默认好友失败", zap.Error(err))
		}
	}
	c.Response(map[string]interface{}{"node_no": nodeNo})
}

// 修改层级
func (a *API) updateNode(c *wkhttp.Context) {
	if err := c.CheckLoginRole(); err != nil {
		c.ResponseError(err)
		return
	}
	nodeNo := c.Param("node_no")
	var req nodeReq
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误"))
		return
	}
	if err := req.check(); err != nil {
		c.ResponseError(err)
		return
	}
	old, err := a.db.queryNode(nodeNo)
	if err != nil || old == nil {
		c.ResponseError(errors.New("层级不存在"))
		return
	}
	conflict, err := a.db.inviteCodeConflict(req.InviteCode, nodeNo)
	if err != nil {
		c.ResponseError(errors.New("校验邀请码冲突失败"))
		return
	}
	if conflict {
		c.ResponseError(errors.New("邀请码已被占用"))
		return
	}
	old.Name = req.Name
	old.OwnerUID = req.OwnerUID
	old.InviteCode = req.InviteCode
	if err := a.db.updateNode(old); err != nil {
		a.Error("更新层级失败", zap.Error(err))
		c.ResponseError(errors.New("更新层级失败"))
		return
	}
	// 默认好友：以请求 list 为准全量替换
	if err := a.db.deleteDefaultFriendsByNode(nodeNo); err != nil {
		a.Warn("清理旧默认好友失败", zap.Error(err))
	}
	for _, uid := range req.DefaultFriendUIDs {
		uid = strings.TrimSpace(uid)
		if uid == "" {
			continue
		}
		if err := a.db.insertDefaultFriend(nodeNo, uid); err != nil {
			a.Warn("写默认好友失败", zap.Error(err))
		}
	}
	c.ResponseOK()
}

// 删除层级（必须无子层级、无用户）
func (a *API) deleteNode(c *wkhttp.Context) {
	if err := c.CheckLoginRole(); err != nil {
		c.ResponseError(err)
		return
	}
	nodeNo := c.Param("node_no")
	n, err := a.db.queryNode(nodeNo)
	if err != nil || n == nil {
		c.ResponseError(errors.New("层级不存在"))
		return
	}
	cc, err := a.db.queryChildrenCount(nodeNo)
	if err != nil {
		c.ResponseError(errors.New("查询子层级失败"))
		return
	}
	if cc > 0 {
		c.ResponseError(errors.New("请先删除子层级"))
		return
	}
	uc, err := a.db.queryUserCount(nodeNo)
	if err != nil {
		c.ResponseError(errors.New("查询层级下用户失败"))
		return
	}
	if uc > 0 {
		c.ResponseError(errors.New("请先将该层级下的用户移走或删除"))
		return
	}
	if err := a.db.deleteDefaultFriendsByNode(nodeNo); err != nil {
		a.Warn("删除默认好友失败", zap.Error(err))
	}
	if err := a.db.deleteNode(nodeNo); err != nil {
		a.Error("删除层级失败", zap.Error(err))
		c.ResponseError(errors.New("删除层级失败"))
		return
	}
	c.ResponseOK()
}

// 层级下用户列表
func (a *API) listUsers(c *wkhttp.Context) {
	if err := c.CheckLoginRole(); err != nil {
		c.ResponseError(err)
		return
	}
	nodeNo := c.Param("node_no")
	keyword := strings.TrimSpace(c.Query("keyword"))
	pageIndex, pageSize := c.GetPage()
	rows, count, err := a.db.queryUsersByNode(nodeNo, keyword, uint64(pageSize), uint64(pageIndex))
	if err != nil {
		a.Error("查询层级用户失败", zap.Error(err))
		c.ResponseError(errors.New("查询层级用户失败"))
		return
	}
	list := make([]*nodeUserResp, 0, len(rows))
	for _, r := range rows {
		registerTime := ""
		if r.CreatedAt.Valid {
			registerTime = r.CreatedAt.Time.Format("2006-01-02 15:04:05")
		}
		list = append(list, &nodeUserResp{
			UID:                    r.UID,
			Name:                   r.Name,
			Phone:                  r.Phone,
			Username:               r.Username,
			ShortNo:                r.ShortNo,
			Status:                 r.Status,
			CanInviteOrCreateGroup: r.CanInviteOrCreateGroup,
			RegisterTime:           registerTime,
			LevelNodeNo:            r.LevelNodeNo,
		})
	}
	c.Response(map[string]interface{}{
		"list":  list,
		"count": count,
	})
}

// 用户搜索（用于选层级负责人 / 默认好友 / 移动用户）
func (a *API) searchUsersAPI(c *wkhttp.Context) {
	if err := c.CheckLoginRole(); err != nil {
		c.ResponseError(err)
		return
	}
	keyword := strings.TrimSpace(c.Query("keyword"))
	limitStr := c.DefaultQuery("limit", "20")
	limit, err := strconv.ParseUint(limitStr, 10, 64)
	if err != nil || limit == 0 || limit > 100 {
		limit = 20
	}
	rows, err := a.db.searchUsers(keyword, limit)
	if err != nil {
		a.Error("搜索用户失败", zap.Error(err))
		c.ResponseError(errors.New("搜索用户失败"))
		return
	}
	list := make([]*nodeUserResp, 0, len(rows))
	for _, r := range rows {
		list = append(list, &nodeUserResp{
			UID:                    r.UID,
			Name:                   r.Name,
			Phone:                  r.Phone,
			Username:               r.Username,
			ShortNo:                r.ShortNo,
			Status:                 r.Status,
			CanInviteOrCreateGroup: r.CanInviteOrCreateGroup,
			LevelNodeNo:            r.LevelNodeNo,
		})
	}
	c.Response(list)
}

// 整个层级一键切换：把指定层级下所有用户的加人/建群权限统一设为 0/1
func (a *API) updateNodePermission(c *wkhttp.Context) {
	if err := c.CheckLoginRole(); err != nil {
		c.ResponseError(err)
		return
	}
	nodeNo := c.Param("node_no")
	if strings.TrimSpace(nodeNo) == "" {
		c.ResponseError(errors.New("层级编号不能为空"))
		return
	}
	var req struct {
		CanInviteOrCreateGroup int `json:"can_invite_or_create_group"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误"))
		return
	}
	val := 0
	if req.CanInviteOrCreateGroup == 1 {
		val = 1
	}
	// 校验层级存在
	n, err := a.db.queryNode(nodeNo)
	if err != nil {
		a.Error("查询层级失败", zap.Error(err))
		c.ResponseError(errors.New("查询层级失败"))
		return
	}
	if n == nil {
		c.ResponseError(errors.New("层级不存在"))
		return
	}
	affected, err := a.db.updateNodeUsersCanInviteOrCreateGroup(nodeNo, val)
	if err != nil {
		a.Error("批量更新层级用户权限失败", zap.Error(err))
		c.ResponseError(errors.New("批量更新层级用户权限失败"))
		return
	}
	c.Response(map[string]interface{}{
		"affected": affected,
	})
}

// 切换用户加人/建群权限
func (a *API) updateUserPermission(c *wkhttp.Context) {
	if err := c.CheckLoginRole(); err != nil {
		c.ResponseError(err)
		return
	}
	var req struct {
		UID                    string `json:"uid"`
		CanInviteOrCreateGroup int    `json:"can_invite_or_create_group"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误"))
		return
	}
	if strings.TrimSpace(req.UID) == "" {
		c.ResponseError(errors.New("用户uid不能为空"))
		return
	}
	val := 0
	if req.CanInviteOrCreateGroup == 1 {
		val = 1
	}
	if err := a.db.updateUserCanInviteOrCreateGroup(req.UID, val); err != nil {
		a.Error("更新用户权限失败", zap.Error(err))
		c.ResponseError(errors.New("更新用户权限失败"))
		return
	}
	c.ResponseOK()
}

// 移动用户：删除旧层级默认好友 + 写新层级默认好友 + 修改 user.level_node_no
func (a *API) moveUser(c *wkhttp.Context) {
	if err := c.CheckLoginRole(); err != nil {
		c.ResponseError(err)
		return
	}
	var req struct {
		UID    string `json:"uid"`
		NodeNo string `json:"node_no"`
		// 可选：传则一并设置目标层级下的加人/建群权限（0/1）。
		// 用于「添加成员」时显式指定新成员的初始权限；不传则保留用户原有权限值。
		CanInviteOrCreateGroup *int `json:"can_invite_or_create_group,omitempty"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误"))
		return
	}
	req.UID = strings.TrimSpace(req.UID)
	req.NodeNo = strings.TrimSpace(req.NodeNo)
	if req.UID == "" {
		c.ResponseError(errors.New("用户uid不能为空"))
		return
	}
	if req.NodeNo == "" {
		c.ResponseError(errors.New("目标层级不能为空"))
		return
	}
	target, err := a.db.queryNode(req.NodeNo)
	if err != nil || target == nil {
		c.ResponseError(errors.New("目标层级不存在"))
		return
	}
	// 查用户当前层级
	var current string
	count, err := a.db.session.Select("level_node_no").From("user").Where("uid=?", req.UID).Limit(1).Load(&current)
	if err != nil {
		a.Error("查询用户层级失败", zap.Error(err))
		c.ResponseError(errors.New("查询用户层级失败"))
		return
	}
	if count == 0 {
		c.ResponseError(errors.New("用户不存在"))
		return
	}
	// 规整一下传入的权限值（容错：除 1 以外都按 0 处理）
	permVal := 0
	if req.CanInviteOrCreateGroup != nil && *req.CanInviteOrCreateGroup == 1 {
		permVal = 1
	}
	// 已在该层级：层级不变，仅按需更新权限
	if current == req.NodeNo {
		if req.CanInviteOrCreateGroup != nil {
			if err := a.db.updateUserCanInviteOrCreateGroup(req.UID, permVal); err != nil {
				a.Error("更新用户权限失败", zap.Error(err))
				c.ResponseError(errors.New("更新用户权限失败"))
				return
			}
		}
		c.ResponseOK()
		return
	}

	tx, err := a.ctx.DB().Begin()
	if err != nil {
		a.Error("开启事务失败", zap.Error(err))
		c.ResponseError(errors.New("开启事务失败"))
		return
	}
	defer func() {
		if e := recover(); e != nil {
			tx.Rollback()
			panic(e)
		}
	}()

	// 1) 删除旧层级默认好友（双向解除）
	if current != "" {
		oldFriends, err := a.db.queryDefaultFriends(current)
		if err != nil {
			tx.Rollback()
			c.ResponseError(errors.New("查询旧层级默认好友失败"))
			return
		}
		for _, fuid := range oldFriends {
			if fuid == req.UID {
				continue
			}
			if err := a.db.removeFriendBidirectional(tx, req.UID, fuid); err != nil {
				tx.Rollback()
				a.Error("解除旧层级默认好友失败", zap.Error(err))
				c.ResponseError(errors.New("解除旧层级默认好友失败"))
				return
			}
		}
	}
	// 2) 修改用户层级（可选附带：把加人/建群权限设为请求值）
	updateMap := map[string]interface{}{"level_node_no": req.NodeNo}
	if req.CanInviteOrCreateGroup != nil {
		updateMap["can_invite_or_create_group"] = permVal
	}
	if _, err := tx.Update("user").SetMap(updateMap).Where("uid=?", req.UID).Exec(); err != nil {
		tx.Rollback()
		a.Error("更新用户层级失败", zap.Error(err))
		c.ResponseError(errors.New("更新用户层级失败"))
		return
	}
	// 3) 加新层级默认好友
	newFriends, err := a.db.queryDefaultFriends(req.NodeNo)
	if err != nil {
		tx.Rollback()
		c.ResponseError(errors.New("查询新层级默认好友失败"))
		return
	}
	for _, fuid := range newFriends {
		if fuid == req.UID {
			continue
		}
		if err := a.db.addFriendBidirectional(tx, req.UID, fuid); err != nil {
			tx.Rollback()
			a.Error("添加新层级默认好友失败", zap.Error(err))
			c.ResponseError(errors.New("添加新层级默认好友失败"))
			return
		}
	}
	if err := tx.Commit(); err != nil {
		tx.Rollback()
		a.Error("提交事务失败", zap.Error(err))
		c.ResponseError(errors.New("提交事务失败"))
		return
	}
	c.ResponseOK()
}

// 把用户移出当前层级：清空 user.level_node_no + 双向解除该层级的默认好友。
// 用户原本就没有层级时直接返回 OK（幂等）。
func (a *API) removeUser(c *wkhttp.Context) {
	if err := c.CheckLoginRole(); err != nil {
		c.ResponseError(err)
		return
	}
	var req struct {
		UID string `json:"uid"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误"))
		return
	}
	req.UID = strings.TrimSpace(req.UID)
	if req.UID == "" {
		c.ResponseError(errors.New("用户uid不能为空"))
		return
	}
	// 查用户当前层级
	var current string
	count, err := a.db.session.Select("level_node_no").From("user").Where("uid=?", req.UID).Limit(1).Load(&current)
	if err != nil {
		a.Error("查询用户层级失败", zap.Error(err))
		c.ResponseError(errors.New("查询用户层级失败"))
		return
	}
	if count == 0 {
		c.ResponseError(errors.New("用户不存在"))
		return
	}
	if current == "" {
		// 已经没有层级，幂等返回
		c.ResponseOK()
		return
	}

	tx, err := a.ctx.DB().Begin()
	if err != nil {
		a.Error("开启事务失败", zap.Error(err))
		c.ResponseError(errors.New("开启事务失败"))
		return
	}
	defer func() {
		if e := recover(); e != nil {
			tx.Rollback()
			panic(e)
		}
	}()

	// 1) 双向解除该层级的默认好友
	friends, err := a.db.queryDefaultFriends(current)
	if err != nil {
		tx.Rollback()
		a.Error("查询默认好友失败", zap.Error(err))
		c.ResponseError(errors.New("查询默认好友失败"))
		return
	}
	for _, fuid := range friends {
		if fuid == "" || fuid == req.UID {
			continue
		}
		if err := a.db.removeFriendBidirectional(tx, req.UID, fuid); err != nil {
			tx.Rollback()
			a.Error("解除默认好友失败", zap.Error(err))
			c.ResponseError(errors.New("解除默认好友失败"))
			return
		}
	}
	// 2) 清空 user.level_node_no
	if _, err := tx.Update("user").Set("level_node_no", "").Where("uid=?", req.UID).Exec(); err != nil {
		tx.Rollback()
		a.Error("更新用户层级失败", zap.Error(err))
		c.ResponseError(errors.New("更新用户层级失败"))
		return
	}
	if err := tx.Commit(); err != nil {
		tx.Rollback()
		a.Error("提交事务失败", zap.Error(err))
		c.ResponseError(errors.New("提交事务失败"))
		return
	}
	c.ResponseOK()
}
