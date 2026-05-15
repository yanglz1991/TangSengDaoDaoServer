package user

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/base/event"
	common2 "github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/common"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/common"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/gin-gonic/gin"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/log"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkevent"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

// Manager 用户管理
type Manager struct {
	ctx *config.Context
	log.Log
	db                *managerDB
	userDB            *DB
	userSettingDB     *SettingDB
	deviceDB          *deviceDB
	friendDB          *friendDB
	loginLogDB        *LoginLogDB
	ipBlacklistDB     *ipBlacklistDB
	deviceBlacklistDB *deviceBlacklistDB
	onlineService     IOnlineService
	commonService     common2.IService
}

// NewManager NewManager
func NewManager(ctx *config.Context) *Manager {
	m := &Manager{
		ctx:               ctx,
		Log:               log.NewTLog("userManager"),
		db:                newManagerDB(ctx),
		deviceDB:          newDeviceDB(ctx),
		friendDB:          newFriendDB(ctx),
		userDB:            NewDB(ctx),
		userSettingDB:     NewSettingDB(ctx.DB()),
		loginLogDB:        NewLoginLogDB(ctx.DB()),
		ipBlacklistDB:     newIPBlacklistDB(ctx),
		deviceBlacklistDB: newDeviceBlacklistDB(ctx),
		onlineService:     NewOnlineService(ctx),
		commonService:     common2.NewService(ctx),
	}
	m.createManagerAccount()
	return m
}

// Route 配置路由规则
func (m *Manager) Route(r *wkhttp.WKHttp) {
	user := r.Group("/v1/manager")
	{
		user.POST("/login", m.login) // 账号登录
	}
	auth := r.Group("/v1/manager", m.ctx.AuthMiddleware(r))
	{
		auth.POST("/user/admin", m.addAdminUser)              // 添加一个管理员
		auth.GET("/user/admin", m.getAdminUsers)              // 查询管理员用户
		auth.DELETE("/user/admin", m.deleteAdminUsers)        // 删除管理员用户
		auth.POST("/user/add", m.addUser)                     // 添加一个用户
		auth.POST("/user/batch_add", m.batchAddUser)          // 批量生成账户（递增手机号）
		auth.POST("/user/resetpassword", m.resetUserPassword) // 重置用户密码
		auth.POST("/user/updatename", m.updateUserName)       // 修改用户昵称
		auth.GET("/user/list", m.list)                        // 用户列表
		auth.GET("/user/friends", m.friends)                  // 某个用户的好友
		auth.GET("/user/blacklist", m.blacklist)              // 用户黑名单列表
		auth.GET("/user/disablelist", m.disableUsers)         // 封禁用户列表
		auth.GET("user/online", m.online)                     // 在线设备信息
		auth.PUT("/user/liftban/:uid/:status", m.liftBanUser) // 解禁或封禁用户
		auth.POST("/user/updatepassword", m.updatePwd)        // 修改用户密码
		auth.GET("/user/devices", m.devices)                  // 查看某用户设备列表

		// 风控：IP 黑名单
		auth.GET("/user/iplist", m.ipList)                         // IP 列表（含关联用户）
		auth.POST("/user/ipblacklist/add", m.ipBlacklistAdd)       // 封禁 IP
		auth.POST("/user/ipblacklist/remove", m.ipBlacklistRemove) // 解禁 IP

		// 风控：设备黑名单
		auth.GET("/user/devicelist", m.deviceList)                         // 设备列表（含关联用户）
		auth.POST("/user/deviceblacklist/add", m.deviceBlacklistAdd)       // 封禁设备
		auth.POST("/user/deviceblacklist/remove", m.deviceBlacklistRemove) // 解禁设备
	}
}

func (m *Manager) devices(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	uid := c.Query("uid")
	if uid == "" {
		c.ResponseError(errors.New("请求用户uid不能为空"))
		return
	}
	devices, err := m.deviceDB.queryDeviceWithUID(uid)
	if err != nil {
		m.Error("查询用户设备列表错误", zap.Error(err))
		c.ResponseError(errors.New("查询用户设备列表错误"))
		return
	}
	list := make([]*managerDeviceResp, 0)
	if len(devices) == 0 {
		c.Response(list)
		return
	}
	for _, device := range devices {
		list = append(list, &managerDeviceResp{
			ID:          device.Id,
			DeviceID:    device.DeviceID,
			DeviceName:  device.DeviceName,
			DeviceModel: device.DeviceModel,
			LastLogin:   util.ToyyyyMMddHHmm(time.Unix(device.LastLogin, 0)),
		})
	}
	c.Response(list)
}

func (m *Manager) online(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	uid := c.Query("uid")
	if uid == "" {
		c.ResponseError(errors.New("请求用户uid不能为空"))
		return
	}
	list, err := m.db.queryUserOnline(uid)
	if err != nil {
		m.Error("查询用户在线设备信息错误", zap.Error(err))
		c.ResponseError(errors.New("查询用户在线设备信息错误"))
		return
	}
	result := make([]*userOnlineResp, 0)
	if len(list) > 0 {
		for _, user := range list {
			result = append(result, &userOnlineResp{
				Online:      user.Online,
				DeviceFlag:  user.DeviceFlag,
				LastOnline:  user.LastOffline,
				LastOffline: user.LastOffline,
				UID:         user.UID,
			})
		}
	}
	c.Response(result)
}

// 用户登录
func (m *Manager) login(c *wkhttp.Context) {
	var req managerLoginReq
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if err := req.Check(); err != nil {
		c.ResponseError(err)
		return
	}
	userInfo, err := m.db.queryUserInfoWithNameAndPwd(req.Username)
	if err != nil {
		m.Error("登录错误", zap.Error(err))
		c.ResponseError(errors.New("登录错误！"))
		return
	}
	if userInfo == nil || userInfo.UID == "" {
		c.ResponseError(errors.New("登录用户不存在"))
		return
	}
	if userInfo.Password != util.MD5(util.MD5(req.Password)) {
		c.ResponseError(errors.New("用户名或密码错误"))
		return
	}
	if userInfo.Role != string(wkhttp.Admin) && userInfo.Role != string(wkhttp.SuperAdmin) {
		c.ResponseError(errors.New("登录账号未开通管理权限"))
		return
	}
	token := util.GenerUUID()
	// 将token设置到缓存
	err = m.ctx.Cache().SetAndExpire(m.ctx.GetConfig().Cache.TokenCachePrefix+token, wkhttp.EncodeTokenCacheInfo(userInfo.UID, userInfo.Name, userInfo.Role), m.ctx.GetConfig().Cache.TokenExpire)
	if err != nil {
		m.Error("设置token缓存失败！", zap.Error(err))
		c.ResponseError(errors.New("设置token缓存失败！"))
		return
	}

	err = m.ctx.Cache().SetAndExpire(fmt.Sprintf("%s%d%s", m.ctx.GetConfig().Cache.UIDTokenCachePrefix, config.Web, userInfo.UID), token, m.ctx.GetConfig().Cache.TokenExpire)
	if err != nil {
		m.Error("设置uidtoken缓存失败！", zap.Error(err))
		c.ResponseError(errors.New("设置token缓存失败！"))
		return
	}

	c.Response(&managerLoginResp{
		UID:   userInfo.UID,
		Token: token,
		Name:  userInfo.Name,
		Role:  userInfo.Role,
	})
}

// 重置用户密码
func (m *Manager) resetUserPassword(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}

	type reqRUP struct {
		NewPassword              string `json:"new_password"`
		NewPassswordConfirmation string `json:"new_password_confirmation"`
		Uid                      string `json:"uid"`
	}
	var req reqRUP
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if len(req.NewPassword) < 6 {
		c.ResponseError(errors.New("密码长度必须大于6位"))
		return
	}
	if req.NewPassword != req.NewPassswordConfirmation {
		c.ResponseError(errors.New("两次密码不一致！"))
		return
	}
	if req.Uid == "" {
		c.ResponseError(errors.New("用户uid不能为空！"))
		return
	}
	user, err := m.userDB.QueryByUID(req.Uid)
	if err != nil {
		m.Error("查询用户信息错误", zap.Error(err))
		c.ResponseError(errors.New("查询用户信息错误"))
		return
	}
	if user == nil {
		c.ResponseError(errors.New("操作用户不存在"))
		return
	}

	err = m.userDB.UpdateUsersWithField("password", util.MD5(util.MD5(req.NewPassword)), req.Uid)
	if err != nil {
		m.Error("重置用户密码错误", zap.Error(err))
		c.Response("重置用户密码错误")
		return
	}
	c.ResponseOK()
}

// 修改用户昵称（超管）
func (m *Manager) updateUserName(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}

	type reqUN struct {
		Uid  string `json:"uid"`
		Name string `json:"name"`
	}
	var req reqUN
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Uid == "" {
		c.ResponseError(errors.New("用户uid不能为空！"))
		return
	}
	if req.Name == "" {
		c.ResponseError(errors.New("昵称不能为空！"))
		return
	}
	if len([]rune(req.Name)) > 20 {
		c.ResponseError(errors.New("昵称长度不能超过20个字符！"))
		return
	}
	user, err := m.userDB.QueryByUID(req.Uid)
	if err != nil {
		m.Error("查询用户信息错误", zap.Error(err))
		c.ResponseError(errors.New("查询用户信息错误"))
		return
	}
	if user == nil {
		c.ResponseError(errors.New("操作用户不存在"))
		return
	}
	if user.Name == req.Name {
		c.ResponseOK()
		return
	}

	err = m.userDB.UpdateUsersWithField("name", req.Name, req.Uid)
	if err != nil {
		m.Error("修改用户昵称错误", zap.Error(err))
		c.ResponseError(errors.New("修改用户昵称错误"))
		return
	}

	// 同步刷新被修改用户在各端的 token 缓存里的 name
	for _, deviceFlag := range []config.DeviceFlag{config.APP, config.Web, config.PC} {
		oldToken, cerr := m.ctx.Cache().Get(fmt.Sprintf("%s%d%s", m.ctx.GetConfig().Cache.UIDTokenCachePrefix, deviceFlag, req.Uid))
		if cerr != nil || oldToken == "" {
			continue
		}
		_ = m.ctx.Cache().Set(m.ctx.GetConfig().Cache.TokenCachePrefix+oldToken, wkhttp.EncodeTokenCacheInfo(req.Uid, req.Name, user.Role))
	}

	// 通知好友刷新该用户频道信息
	friends, err := m.friendDB.QueryFriends(req.Uid)
	if err != nil {
		m.Error("查询用户好友错误", zap.Error(err))
		c.ResponseOK()
		return
	}
	if len(friends) > 0 {
		uids := make([]string, 0, len(friends)+1)
		uids = append(uids, req.Uid)
		for _, friend := range friends {
			uids = append(uids, friend.ToUID)
		}
		err = m.ctx.SendCMD(config.MsgCMDReq{
			CMD:         common.CMDChannelUpdate,
			Subscribers: uids,
			Param: map[string]interface{}{
				"channel_id":   req.Uid,
				"channel_type": common.ChannelTypePerson,
			},
		})
		if err != nil {
			m.Error("发送频道更改消息错误！", zap.Error(err))
		}
	}
	c.ResponseOK()
}

// 删除管理员用户
func (m *Manager) deleteAdminUsers(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	uid := c.Query("uid")
	if uid == "" {
		c.ResponseError(errors.New("删除用户uid不能为空"))
		return
	}
	user, err := m.userDB.QueryByUID(uid)
	if err != nil {
		m.Error("查询管理员用户错误", zap.Error(err))
		c.ResponseError(errors.New("查询管理员用户错误"))
		return
	}
	if user == nil || len(user.UID) == 0 {
		c.ResponseError(errors.New("该用户不存在"))
		return
	}
	if user.Role == "" {
		c.ResponseError(errors.New("该用户不是管理员账号不能删除"))
		return
	}
	if user.Role == string(wkhttp.SuperAdmin) {
		c.ResponseError(errors.New("超级管理员账号不能删除"))
		return
	}
	err = m.db.deleteUserWithUIDAndRole(uid, string(wkhttp.Admin))
	if err != nil {
		m.Error("删除管理员错误", zap.Error(err))
		c.ResponseError(errors.New("删除管理员错误"))
		return
	}
	oldToken, err := m.ctx.Cache().Get(fmt.Sprintf("%s%d%s", m.ctx.GetConfig().Cache.UIDTokenCachePrefix, config.Web, user.UID))
	if err != nil {
		m.Error("获取旧token错误", zap.Error(err))
		c.ResponseError(errors.New("获取旧token错误"))
		return
	}
	if oldToken != "" {
		err = m.ctx.Cache().Delete(m.ctx.GetConfig().Cache.TokenCachePrefix + oldToken)
		if err != nil {
			m.Error("清除旧token数据错误", zap.Error(err))
			c.ResponseError(errors.New("清除旧token数据错误"))
			return
		}
	}
	c.ResponseOK()
}

// 查询管理员列表
func (m *Manager) getAdminUsers(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	users, err := m.db.queryUsersWithRole(string(wkhttp.Admin))
	if err != nil {
		m.Error("查询管理员用户错误", zap.Error(err))
		c.ResponseError(errors.New("查询管理员用户错误"))
		return
	}
	list := make([]*adminUserResp, 0)
	if len(users) > 0 {
		for _, user := range users {
			list = append(list, &adminUserResp{
				UID:          user.UID,
				Name:         user.Name,
				Username:     user.Username,
				RegisterTime: user.CreatedAt.String(),
			})
		}
	}
	c.Response(list)
}

// 添加一个管理员
func (m *Manager) addAdminUser(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type reqVO struct {
		LoginName string `json:"login_name"`
		Name      string `json:"name"`
		Password  string `json:"password"`
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if req.LoginName == "" {
		c.ResponseError(errors.New("登录用户名不能为空"))
		return
	}
	if req.Name == "" {
		c.ResponseError(errors.New("用户名不能为空"))
		return
	}
	if req.Password == "" {
		c.ResponseError(errors.New("密码不能为空"))
		return
	}
	user, err := m.db.queryUserWithNameAndRole(req.LoginName, string(wkhttp.Admin))
	if err != nil {
		m.Error("查询用户是否存在错误", zap.String("username", req.LoginName))
		c.ResponseError(errors.New("查询用户是否存在错误"))
		return
	}
	if user != nil && len(user.UID) > 0 {
		c.ResponseError(errors.New("该用户名已存在"))
		return
	}
	userModel := &Model{}
	userModel.UID = util.GenerUUID()
	userModel.Name = req.Name
	userModel.Vercode = fmt.Sprintf("%s@%d", util.GenerUUID(), common.User)
	userModel.QRVercode = fmt.Sprintf("%s@%d", util.GenerUUID(), common.QRCode)
	userModel.Phone = ""
	userModel.Username = req.LoginName
	userModel.Zone = ""
	userModel.Role = string(wkhttp.Admin)
	userModel.Password = util.MD5(util.MD5(req.Password))
	userModel.ShortNo = util.Ten2Hex(time.Now().UnixNano())
	userModel.IsUploadAvatar = 0
	userModel.NewMsgNotice = 0
	userModel.MsgShowDetail = 0
	userModel.SearchByPhone = 0
	userModel.SearchByShort = 0
	userModel.VoiceOn = 0
	userModel.ShockOn = 0
	userModel.Sex = 1
	userModel.Status = int(common.UserAvailable)
	err = m.userDB.Insert(userModel)
	if err != nil {
		m.Error("添加管理员错误", zap.String("username", req.Name))
		c.ResponseError(err)
		return
	}
	c.ResponseOK()
}

// 添加一个用户
func (m *Manager) addUser(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	var req managerAddUserReq
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if err := req.checkAddUserReq(); err != nil {
		c.ResponseError(err)
		return
	}
	userInfo, err := m.userDB.QueryByUsername(fmt.Sprintf("%s%s", req.Zone, req.Phone))
	if err != nil {
		m.Error("查询用户信息失败！", zap.String("username", req.Phone))
		c.ResponseError(err)
		return
	}
	if userInfo != nil {
		c.ResponseError(errors.New("该用户已存在"))
		return
	}
	uid := util.GenerUUID()
	var shortNo = ""
	var shortNumStatus = 0
	if m.ctx.GetConfig().ShortNo.NumOn {
		shortNo, err = m.commonService.GetShortno()
		if err != nil {
			m.Error("获取短编号失败！", zap.Error(err))
			c.ResponseError(errors.New("获取短编号失败！"))
			return
		}
	} else {
		shortNo = util.Ten2Hex(time.Now().UnixNano())
	}
	if m.ctx.GetConfig().ShortNo.EditOff {
		shortNumStatus = 1
	}
	tx, err := m.db.session.Begin()
	if err != nil {
		m.Error("开启事物错误", zap.Error(err))
		c.ResponseError(errors.New("开启事物错误"))
		return
	}
	defer func() {
		if err := recover(); err != nil {
			tx.Rollback()
			panic(err)
		}
	}()
	userModel := &Model{}
	userModel.UID = uid
	userModel.Name = req.Name
	userModel.Vercode = fmt.Sprintf("%s@%d", util.GenerUUID(), common.User)
	userModel.QRVercode = fmt.Sprintf("%s@%d", util.GenerUUID(), common.QRCode)
	userModel.Phone = req.Phone
	userModel.Username = fmt.Sprintf("%s%s", req.Zone, req.Phone)
	userModel.Zone = req.Zone
	userModel.Password = util.MD5(util.MD5(req.Password))
	userModel.ShortNo = shortNo
	userModel.IsUploadAvatar = 0
	userModel.NewMsgNotice = 1
	userModel.MsgShowDetail = 1
	userModel.SearchByPhone = 1
	userModel.ShortStatus = shortNumStatus
	userModel.SearchByShort = 1
	userModel.VoiceOn = 1
	userModel.ShockOn = 1
	userModel.Sex = req.Sex
	// 「加好友 / 创建群聊」权限：默认 0（禁止），管理员添加时可显式开启。
	if req.CanInviteOrCreateGroup == 1 {
		userModel.CanInviteOrCreateGroup = 1
	}
	userModel.Status = int(common.UserAvailable)
	err = m.userDB.insertTx(userModel, tx)
	if err != nil {
		tx.Rollback()
		m.Error("添加用户错误", zap.String("username", req.Phone))
		c.ResponseError(err)
		return
	}

	err = m.addSystemFriend(uid)
	if err != nil {
		tx.Rollback()
		c.ResponseError(errors.New("添加后台生成用户和系统账号为好友关系失败"))
		return
	}
	err = m.addFileHelperFriend(uid)
	if err != nil {
		tx.Rollback()
		c.ResponseError(errors.New("添加后台生成用户和文件助手为好友关系失败"))
		return
	}
	//发送用户注册事件
	eventID, err := m.ctx.EventBegin(&wkevent.Data{
		Event: event.EventUserRegister,
		Type:  wkevent.Message,
		Data: map[string]interface{}{
			"uid": uid,
		},
	}, tx)
	if err != nil {
		tx.RollbackUnlessCommitted()
		m.Error("开启事件失败！", zap.Error(err))
		c.ResponseError(errors.New("开启事件失败！"))
		return
	}
	err = tx.Commit()
	if err != nil {
		tx.Rollback()
		m.Error("数据库事物提交失败", zap.Error(err))
		c.ResponseError(errors.New("数据库事物提交失败"))
		return
	}
	m.ctx.EventCommit(eventID)
	c.ResponseOK()
}

// batchAddUser 批量生成账户（基于起始手机号递增）
//
// 典型用法：
//
//	start_phone=15500000000 count=100 -> 生成 15500000000 ~ 15500000099 共 100 条
//
// 字段：
//
//	start_phone (必填)             起始手机号，必须是 11 位数字字符串
//	count       (必填, 1~1000)     生成数量
//	zone        (可选, 默认 0086)   手机号区号
//	password    (可选)             登录密码，留空则使用 phone 自身
//	sex         (可选, 默认 1)
//	can_invite_or_create_group (可选 0/1, 默认 0)  是否开启「加好友/创建群聊」权限
//
// 返回：
//
//	success: 成功条数
//	skipped: 已存在跳过的手机号 [{phone, reason}]
//	failed:  失败的手机号列表 [{phone, reason}]
func (m *Manager) batchAddUser(c *wkhttp.Context) {
	if err := c.CheckLoginRoleIsSuperAdmin(); err != nil {
		c.ResponseError(err)
		return
	}
	var req struct {
		StartPhone             string `json:"start_phone"`
		Count                  int    `json:"count"`
		Zone                   string `json:"zone"`
		Password               string `json:"password"`
		Sex                    int    `json:"sex"`
		CanInviteOrCreateGroup int    `json:"can_invite_or_create_group"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}

	// 输入校验
	startPhone := strings.TrimSpace(req.StartPhone)
	if len(startPhone) != 11 {
		c.ResponseError(errors.New("起始手机号必须是 11 位数字"))
		return
	}
	startNum, err := strconv.ParseInt(startPhone, 10, 64)
	if err != nil {
		c.ResponseError(errors.New("起始手机号必须是 11 位数字"))
		return
	}
	if req.Count <= 0 {
		c.ResponseError(errors.New("生成数量必须大于 0"))
		return
	}
	const maxBatch = 1000
	if req.Count > maxBatch {
		c.ResponseError(fmt.Errorf("单次最多生成 %d 个账户", maxBatch))
		return
	}
	zone := req.Zone
	if zone == "" {
		zone = "0086"
	}
	sex := req.Sex
	if sex != 0 && sex != 1 {
		sex = 1
	}

	type batchItem struct {
		Phone  string `json:"phone"`
		Reason string `json:"reason,omitempty"`
		UID    string `json:"uid,omitempty"`
	}
	skipped := make([]batchItem, 0)
	failed := make([]batchItem, 0)
	successList := make([]batchItem, 0)

	// 一一生成；每个用户独立事务，避免单条失败影响整体。
	for i := 0; i < req.Count; i++ {
		phone := fmt.Sprintf("%011d", startNum+int64(i))

		// 已存在则跳过
		existed, qErr := m.userDB.QueryByUsername(fmt.Sprintf("%s%s", zone, phone))
		if qErr != nil {
			failed = append(failed, batchItem{Phone: phone, Reason: "查询是否已存在失败"})
			m.Error("批量生成用户：查询已存在失败", zap.Error(qErr), zap.String("phone", phone))
			continue
		}
		if existed != nil {
			skipped = append(skipped, batchItem{Phone: phone, Reason: "该手机号已存在"})
			continue
		}

		// 生成短编号
		var shortNo string
		var shortNumStatus int
		if m.ctx.GetConfig().ShortNo.NumOn {
			shortNo, err = m.commonService.GetShortno()
			if err != nil {
				failed = append(failed, batchItem{Phone: phone, Reason: "获取短编号失败"})
				continue
			}
		} else {
			shortNo = util.Ten2Hex(time.Now().UnixNano())
		}
		if m.ctx.GetConfig().ShortNo.EditOff {
			shortNumStatus = 1
		}

		uid := util.GenerUUID()

		// 名称/密码默认值（与用户原话一致：默认都是手机号本身）
		name := phone
		password := req.Password
		if strings.TrimSpace(password) == "" {
			password = phone
		}

		tx, txErr := m.db.session.Begin()
		if txErr != nil {
			failed = append(failed, batchItem{Phone: phone, Reason: "开启事务失败"})
			m.Error("批量生成用户：开启事务失败", zap.Error(txErr), zap.String("phone", phone))
			continue
		}

		userModel := &Model{}
		userModel.UID = uid
		userModel.Name = name
		userModel.Vercode = fmt.Sprintf("%s@%d", util.GenerUUID(), common.User)
		userModel.QRVercode = fmt.Sprintf("%s@%d", util.GenerUUID(), common.QRCode)
		userModel.Phone = phone
		userModel.Username = fmt.Sprintf("%s%s", zone, phone)
		userModel.Zone = zone
		userModel.Password = util.MD5(util.MD5(password))
		userModel.ShortNo = shortNo
		userModel.IsUploadAvatar = 0
		userModel.NewMsgNotice = 1
		userModel.MsgShowDetail = 1
		userModel.SearchByPhone = 1
		userModel.ShortStatus = shortNumStatus
		userModel.SearchByShort = 1
		userModel.VoiceOn = 1
		userModel.ShockOn = 1
		userModel.Sex = sex
		if req.CanInviteOrCreateGroup == 1 {
			userModel.CanInviteOrCreateGroup = 1
		}
		userModel.Status = int(common.UserAvailable)

		if err := m.userDB.insertTx(userModel, tx); err != nil {
			tx.Rollback()
			failed = append(failed, batchItem{Phone: phone, Reason: "插入用户记录失败"})
			m.Error("批量生成用户：插入失败", zap.Error(err), zap.String("phone", phone))
			continue
		}
		// 与系统账号 / 文件助手互为好友（与 addUser 行为保持一致）
		if err := m.addSystemFriend(uid); err != nil {
			tx.Rollback()
			failed = append(failed, batchItem{Phone: phone, Reason: "添加系统好友失败"})
			continue
		}
		if err := m.addFileHelperFriend(uid); err != nil {
			tx.Rollback()
			failed = append(failed, batchItem{Phone: phone, Reason: "添加文件助手好友失败"})
			continue
		}
		// 用户注册事件（friend 模块依赖此事件清理 vercode 等）
		eventID, evErr := m.ctx.EventBegin(&wkevent.Data{
			Event: event.EventUserRegister,
			Type:  wkevent.Message,
			Data: map[string]interface{}{
				"uid": uid,
			},
		}, tx)
		if evErr != nil {
			tx.RollbackUnlessCommitted()
			failed = append(failed, batchItem{Phone: phone, Reason: "开启用户注册事件失败"})
			continue
		}
		if err := tx.Commit(); err != nil {
			tx.Rollback()
			failed = append(failed, batchItem{Phone: phone, Reason: "提交事务失败"})
			continue
		}
		m.ctx.EventCommit(eventID)
		successList = append(successList, batchItem{Phone: phone, UID: uid})
	}

	c.JSON(http.StatusOK, gin.H{
		"total":   req.Count,
		"success": len(successList),
		"skipped": skipped,
		"failed":  failed,
		"items":   successList,
	})
}

// 用户列表
func (m *Manager) list(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	keyword := c.Query("keyword")
	onlineStr := c.Query("online")

	var online int64 = -1
	if strings.TrimSpace(onlineStr) != "" {
		online, _ = strconv.ParseInt(onlineStr, 10, 64)
	}
	pageIndex, pageSize := c.GetPage()
	var userList []*managerUserModel
	var count int64
	if keyword == "" {
		userList, err = m.db.queryUserListWithPage(uint64(pageSize), uint64(pageIndex), int(online))
		if err != nil {
			m.Error("查询用户列表报错", zap.Error(err))
			c.ResponseError(err)
			return
		}

		count, err = m.userDB.queryUserCount()
		if err != nil {
			m.Error("查询用户数量错误", zap.Error(err))
			c.ResponseError(errors.New("查询用户数量错误"))
			return
		}
	} else {
		userList, err = m.db.queryUserListWithPageAndKeyword(keyword, int(online), uint64(pageSize), uint64(pageIndex))
		if err != nil {
			m.Error("查询用户列表报错", zap.Error(err))
			c.ResponseError(err)
			return
		}

		count, err = m.db.queryUserCountWithKeyWord(keyword)
		if err != nil {
			m.Error("查询用户数量错误", zap.Error(err))
			c.ResponseError(errors.New("查询用户数量错误"))
			return
		}
	}

	result := make([]*managerUserResp, 0)
	if len(userList) > 0 {
		uids := make([]string, 0)
		for _, user := range userList {
			uids = append(uids, user.UID)
		}
		resps, err := m.onlineService.GetUserLastOnlineStatus(uids)
		respsdata := map[string]*config.OnlinestatusResp{}
		if len(resps) > 0 {
			for _, v := range resps {
				respsdata[v.UID] = v
			}
		}
		if err != nil {
			m.Error("查询用户在线状态失败", zap.Error(err))
			c.ResponseError(errors.New("查询用户在线状态失败"))
			return
		}
		devices, err := m.deviceDB.queryDeviceLastLoginWithUids(uids)
		if err != nil {
			m.Error("查询用户最后一次登录设备信息错误", zap.Error(err))
			c.ResponseError(errors.New("查询用户最后一次登录设备信息错误"))
			return
		}
		loginLogs, err := m.loginLogDB.queryLastLoginIPWithUids(uids)
		if err != nil {
			m.Error("查询用户最后一次登录IP错误", zap.Error(err))
			c.ResponseError(errors.New("查询用户最后一次登录IP错误"))
			return
		}
		var i = 0
		for _, user := range userList {
			var device *deviceModel
			if len(devices) > 0 {
				for _, model := range devices {
					if model.UID == user.UID {
						device = model
						break
					}
				}
			}
			var lastLoginIP string = ""
			if len(loginLogs) > 0 {
				for _, lg := range loginLogs {
					if lg.UID == user.UID {
						lastLoginIP = lg.LoginIP
						break
					}
				}
			}
			var lastLoginTime string
			var deviceName string = ""
			var deviceModel string = ""
			var online int
			var lastOnlineTime string = ""
			if device != nil {
				deviceModel = device.DeviceModel
				deviceName = device.DeviceName
				lastLoginTime = util.ToyyyyMMddHHmm(time.Unix(device.LastLogin, 0))
			}
			/* if i < len(resps) {
				online = resps[i].Online
				lastOnlineTime = util.ToyyyyMMddHHmm(time.Unix(int64(resps[i].LastOffline), 0))
			} */
			if respsdata[user.UID] != nil {
				online = respsdata[user.UID].Online
				lastOnlineTime = util.ToyyyyMMddHHmm(time.Unix(int64(respsdata[user.UID].LastOffline), 0))
			}
			showPhone := getShowPhoneNum(user.Phone)
			result = append(result, &managerUserResp{
				UID:                    user.UID,
				Username:               user.Username,
				Name:                   user.Name,
				Phone:                  showPhone,
				Sex:                    user.Sex,
				ShortNo:                user.ShortNo,
				LastLoginTime:          lastLoginTime,
				LastLoginIP:            lastLoginIP,
				DeviceName:             deviceName,
				DeviceModel:            deviceModel,
				Online:                 online,
				LastOnlineTime:         lastOnlineTime,
				RegisterTime:           user.CreatedAt.String(),
				Status:                 user.Status,
				IsDestroy:              user.IsDestroy,
				GiteeUID:               user.GiteeUID,
				GithubUID:              user.GithubUID,
				WXOpenid:               user.WXOpenid,
				CanInviteOrCreateGroup: user.CanInviteOrCreateGroup,
			})
			i++
		}
	}
	c.Response(map[string]interface{}{
		"list":  result,
		"count": count,
	})
}

// 查询某个用户的好友
func (m *Manager) friends(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	uid := c.Query("uid")
	if uid == "" {
		c.ResponseError(errors.New("查询用户ID不能为空"))
		return
	}
	sortType := c.Query("sort_type")
	if sortType == "" {
		sortType = "1"
	}
	sortTypeInt, err := strconv.Atoi(sortType)
	if err != nil {
		sortTypeInt = 0
	}
	list, err := m.friendDB.QueryFriends(uid)
	if err != nil {
		m.Error("查询用户好友错误", zap.String("uid", uid))
		c.ResponseError(err)
		return
	}
	result := make([]*managerFriendResp, 0)
	if len(list) == 0 {
		c.Response(result)
		return
	}
	if sortTypeInt == 0 {
		for _, friend := range list {
			result = append(result, &managerFriendResp{
				UID:              friend.ToUID,
				Remark:           friend.Remark,
				Name:             friend.ToName,
				RelationshipTime: friend.CreatedAt.String(),
			})
		}
		c.Response(result)
		return
	}
	// 查询最近会话
	conversations, err := m.ctx.IMSyncUserConversation(uid, 0, 1, "", nil)
	if err != nil {
		m.Error("同步离线后的最近会话失败！", zap.Error(err), zap.String("loginUID", uid))
		c.ResponseError(errors.New("同步离线后的最近会话失败！"))
		return
	}
	if len(conversations) == 0 {
		for _, friend := range list {
			result = append(result, &managerFriendResp{
				UID:              friend.ToUID,
				Remark:           friend.Remark,
				Name:             friend.ToName,
				RelationshipTime: friend.CreatedAt.String(),
			})
		}
		c.Response(result)
		return
	}
	sort.SliceStable(conversations, func(i, j int) bool {
		return conversations[i].Timestamp > conversations[j].Timestamp
	})
	for _, conv := range conversations {
		if conv.ChannelType != common.ChannelTypePerson.Uint8() {
			continue
		}
		var f *DetailModel
		for _, friend := range list {
			if friend.ToUID == conv.ChannelID {
				f = friend
				break
			}
		}
		if f != nil {
			result = append(result, &managerFriendResp{
				UID:              f.ToUID,
				Remark:           f.Remark,
				Name:             f.ToName,
				RelationshipTime: f.CreatedAt.String(),
			})
		}
	}
	for _, f := range list {
		isAdd := true
		for _, r := range result {
			if r.UID == f.ToUID {
				isAdd = false
				break
			}
		}
		if isAdd {
			result = append(result, &managerFriendResp{
				UID:              f.ToUID,
				Remark:           f.Remark,
				Name:             f.ToName,
				RelationshipTime: f.CreatedAt.String(),
			})
		}
	}
	c.Response(result)
}

// 查询某个用户的黑名单
func (m *Manager) blacklist(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	uid := c.Query("uid")
	if uid == "" {
		c.ResponseError(errors.New("查询用户ID不能为空"))
		return
	}
	list, err := m.db.queryUserBlacklists(uid)
	if err != nil {
		m.Error("查询黑名单列表失败！", zap.Error(err))
		c.ResponseError(errors.New("查询黑名单列表失败！"))
		return
	}
	blacklists := []*managerBlackUserResp{}
	for _, result := range list {
		blacklists = append(blacklists, &managerBlackUserResp{
			UID:      result.UID,
			Name:     result.Name,
			CreateAt: result.UpdatedAt.String(),
		})
	}
	c.Response(blacklists)
}

// 查看封禁用户列表
func (m *Manager) disableUsers(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	pageIndex, pageSize := c.GetPage()
	list, err := m.db.queryUserListWithStatus(int(common.UserDisable), uint64(pageSize), uint64(pageIndex))
	if err != nil {
		m.Error("通过状态查询用户列表错误", zap.Error(err))
		c.ResponseError(errors.New("通过状态查询用户列表错误"))
		return
	}
	count, err := m.db.queryUserCountWithStatus(int(common.UserDisable))
	if err != nil {
		m.Error("查询用户数量错误", zap.Error(err))
		c.ResponseError(errors.New("查询用户数量错误"))
		return
	}
	result := make([]*managerDisableUserResp, 0)
	if len(list) > 0 {
		for _, user := range list {
			showPhone := getShowPhoneNum(user.Phone)
			result = append(result, &managerDisableUserResp{
				Name:         user.Name,
				ShortNo:      user.ShortNo,
				Phone:        showPhone,
				UID:          user.UID,
				ClosureTime:  user.UpdatedAt.String(),
				RegisterTime: user.CreatedAt.String(),
			})
		}
	}
	c.Response(map[string]interface{}{
		"list":  result,
		"count": count,
	})
}

// 封禁或解禁用户
func (m *Manager) liftBanUser(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	uid := c.Param("uid")
	status := c.Param("status")
	if uid == "" {
		c.ResponseError(errors.New("操作用户id不能为空"))
		return
	}
	if status == "" {
		c.ResponseError(errors.New("修改状态类型不能为空"))
		return
	}
	userStatus, _ := strconv.Atoi(status)
	if userStatus != int(common.UserAvailable) && userStatus != int(common.UserDisable) {
		c.ResponseError(errors.New("修改状态类型不匹配"))
		return
	}
	userInfo, err := m.userDB.QueryByUID(uid)
	if err != nil {
		m.Error("查询用户信息失败！", zap.String("uid", uid))
		c.ResponseError(errors.New("查询用户信息错误"))
		return
	}
	if userInfo == nil {
		c.ResponseError(errors.New("操作用户不存在"))
		return
	}
	if userInfo.Status == userStatus {
		c.ResponseOK()
		return
	}
	err = m.userDB.UpdateUsersWithField("status", status, uid)
	if err != nil {
		m.Error("修改用户状态错误", zap.Error(err))
		c.ResponseError(errors.New("修改用户状态错误"))
		return
	}

	ban := 0
	if userStatus == int(common.UserDisable) {
		ban = 1
	}

	err = m.ctx.IMCreateOrUpdateChannelInfo(&config.ChannelInfoCreateReq{
		ChannelID:   uid,
		ChannelType: common.ChannelTypePerson.Uint8(),
		Ban:         ban,
	})
	if err != nil {
		m.Error("更新WebIM的token失败！", zap.Error(err))
		c.ResponseError(errors.New("更新IM的token失败！"))
		return
	}
	// 封禁场景：先推送 forceLogout CMD 让客户端弹窗显示原因，再切断长连
	// sendForceLogoutCMD 内部已留出 CMD 处理时间，调用返回后再 QuitUserDevice 即可
	if userStatus == int(common.UserDisable) {
		m.sendForceLogoutCMD([]string{userInfo.UID}, "您的账号已被管理员封禁", ForceLogoutMatchUser, userInfo.UID)
	}
	err = m.ctx.QuitUserDevice(userInfo.UID, -1)
	if err != nil {
		m.Error("下线用户所有登录设备错误", zap.Error(err), zap.String("uid", uid))
		c.ResponseError(errors.New("下线用户所有登录设备错误"))
		return
	}
	c.ResponseOK()
}

// 修改登录密码
func (m *Manager) updatePwd(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	loginUID := c.GetLoginUID()
	type updatePwdReq struct {
		Password    string `json:"password"`
		NewPassword string `json:"new_password"`
	}
	var req updatePwdReq
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if req.Password == "" || req.NewPassword == "" {
		c.ResponseError(errors.New("密码不能为空"))
		return
	}
	user, err := m.userDB.QueryByUID(loginUID)
	if err != nil {
		m.Error("查询用户信息错误", zap.Error(err))
		c.ResponseError(errors.New("查询用户信息错误"))
		return
	}
	if user == nil {
		c.ResponseError(errors.New("操作用户不存在"))
		return
	}
	if util.MD5(util.MD5(req.Password)) != user.Password {
		c.ResponseError(errors.New("原密码错误"))
		return
	}
	if len(req.NewPassword) < 6 {
		c.ResponseError(errors.New("密码长度必须大于6位"))
		return
	}
	if req.Password == req.NewPassword {
		c.ResponseError(errors.New("新密码不能和旧密码一样"))
		return
	}
	err = m.userDB.UpdateUsersWithField("password", util.MD5(util.MD5(req.NewPassword)), loginUID)
	if err != nil {
		m.Error("修改用户密码错误", zap.Error(err))
		c.Response("修改用户密码错误")
		return
	}
	// 清除token缓存
	oldToken, err := m.ctx.Cache().Get(fmt.Sprintf("%s%d%s", m.ctx.GetConfig().Cache.UIDTokenCachePrefix, config.Web, user.UID))
	if err != nil {
		m.Error("获取旧token错误", zap.Error(err))
		c.ResponseError(errors.New("获取旧token错误"))
		return
	}
	if oldToken != "" {
		err = m.ctx.Cache().Delete(m.ctx.GetConfig().Cache.TokenCachePrefix + oldToken)
		if err != nil {
			m.Error("清除旧token数据错误", zap.Error(err))
			c.ResponseError(errors.New("清除旧token数据错误"))
			return
		}
	}
	c.ResponseOK()
}
func (r managerAddUserReq) checkAddUserReq() error {
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("用户名不能为空！")
	}
	if strings.TrimSpace(r.Password) == "" {
		return errors.New("密码不能为空！")
	}
	if strings.TrimSpace(r.Phone) == "" {
		return errors.New("手机号不能为空！")
	}

	return nil
}
func (r managerLoginReq) Check() error {
	if strings.TrimSpace(r.Username) == "" {
		return errors.New("用户名不能为空！")
	}
	if strings.TrimSpace(r.Password) == "" {
		return errors.New("密码不能为空！")
	}
	return nil
}

// 处理注册用户和文件助手互为好友
func (m *Manager) addFileHelperFriend(uid string) error {
	if uid == "" {
		m.Error("用户ID不能为空")
		return errors.New("用户ID不能为空")
	}
	isFriend, err := m.friendDB.IsFriend(uid, m.ctx.GetConfig().Account.FileHelperUID)
	if err != nil {
		m.Error("查询用户关系失败")
		return err
	}
	if !isFriend {
		version := m.ctx.GenSeq(common.FriendSeqKey)
		err := m.friendDB.Insert(&FriendModel{
			UID:     uid,
			ToUID:   m.ctx.GetConfig().Account.FileHelperUID,
			Version: version,
		})
		if err != nil {
			m.Error("注册用户和文件助手成为好友失败")
			return err
		}
	}
	return nil
}

// addSystemFriend 处理注册用户和系统账号互为好友
func (m *Manager) addSystemFriend(uid string) error {

	if uid == "" {
		m.Error("用户ID不能为空")
		return errors.New("用户ID不能为空")
	}
	isFriend, err := m.friendDB.IsFriend(uid, m.ctx.GetConfig().Account.SystemUID)
	if err != nil {
		m.Error("查询用户关系失败")
		return err
	}
	tx, err := m.friendDB.session.Begin()
	if err != nil {
		m.Error("开启事物错误", zap.Error(err))
		return errors.New("开启事物错误")
	}
	defer func() {
		if err := recover(); err != nil {
			tx.Rollback()
			panic(err)
		}
	}()
	if !isFriend {
		version := m.ctx.GenSeq(common.FriendSeqKey)
		err := m.friendDB.InsertTx(&FriendModel{
			UID:     uid,
			ToUID:   m.ctx.GetConfig().Account.SystemUID,
			Version: version,
		}, tx)
		if err != nil {
			m.Error("注册用户和系统账号成为好友失败")
			tx.Rollback()
			return err
		}
	}
	// systemIsFriend, err := u.friendDB.IsFriend(u.ctx.GetConfig().SystemUID, uid)
	// if err != nil {
	// 	u.Error("查询系统账号和注册用户关系失败")
	// 	tx.Rollback()
	// 	return err
	// }
	// if !systemIsFriend {
	// 	version := u.ctx.GenSeq(common.FriendSeqKey)
	// 	err := u.friendDB.InsertTx(&FriendModel{
	// 		UID:     u.ctx.GetConfig().SystemUID,
	// 		ToUID:   uid,
	// 		Version: version,
	// 	}, tx)
	// 	if err != nil {
	// 		u.Error("系统账号和注册用户成为好友失败")
	// 		tx.Rollback()
	// 		return err
	// 	}
	// }
	err = tx.Commit()
	if err != nil {
		tx.Rollback()
		m.Error("用户注册数据库事物提交失败", zap.Error(err))
		return err
	}
	return nil
}

// 创建一个系统管理账户
func (m *Manager) createManagerAccount() {
	user, err := m.userDB.QueryByUID(m.ctx.GetConfig().Account.AdminUID)
	if err != nil {
		m.Error("查询系统管理账号错误", zap.Error(err))
		return
	}
	if (user != nil && user.UID != "") || m.ctx.GetConfig().AdminPwd == "" {
		return
	}

	username := string(wkhttp.SuperAdmin)
	role := string(wkhttp.SuperAdmin)
	var pwd = m.ctx.GetConfig().AdminPwd
	err = m.userDB.Insert(&Model{
		UID:      m.ctx.GetConfig().Account.AdminUID,
		Name:     "超级管理员",
		ShortNo:  "30000",
		Category: "system",
		Role:     role,
		Username: username,
		Zone:     "0086",
		Phone:    "13000000002",
		Status:   1,
		Password: util.MD5(util.MD5(pwd)),
	})
	if err != nil {
		m.Error("新增系统管理员错误", zap.Error(err))
		return
	}
}
func getShowPhoneNum(mobile string) string {
	if len(mobile) <= 3 {
		return mobile
	}
	phone := mobile[:3]
	var length = len(mobile) - 3
	if length > 4 {
		length = 4
	}
	for i := 0; i < length; i++ {
		phone = fmt.Sprintf("%s*", phone)
	}
	var index = 3 + length
	if index > 0 && index < len(mobile) {
		return phone + mobile[index:]
	}
	return phone
}

type managerLoginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type managerLoginResp struct {
	UID   string `json:"uid"`
	Token string `json:"token"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}
type managerAddUserReq struct {
	Name                   string `json:"name"`
	Password               string `json:"password"`
	Phone                  string `json:"phone"`
	Zone                   string `json:"zone"`
	Sex                    int    `json:"sex"`
	CanInviteOrCreateGroup int    `json:"can_invite_or_create_group"` // 是否允许主动加好友/创建群聊 0.否 1.是
}
type managerBlackUserResp struct {
	Name     string `json:"name"`
	UID      string `json:"uid"`
	CreateAt string `json:"create_at"`
}
type adminUserResp struct {
	Name         string `json:"name"`
	UID          string `json:"uid"`
	Username     string `json:"username"`
	RegisterTime string `json:"register_time"`
}
type managerUserResp struct {
	Name           string `json:"name"`
	UID            string `json:"uid"`
	Phone          string `json:"phone"`
	Username       string `json:"username"`
	ShortNo        string `json:"short_no"`
	Sex            int    `json:"sex"`
	RegisterTime   string `json:"register_time"`
	LastLoginTime  string `json:"last_login_time"`
	LastLoginIP    string `json:"last_login_ip"`
	DeviceName     string `json:"device_name"`
	DeviceModel    string `json:"device_model"`
	Online         int    `json:"online"`
	LastOnlineTime string `json:"last_online_time"`
	Status         int    `json:"status"`
	IsDestroy      int    `json:"is_destroy"`
	WXOpenid       string `json:"wx_openid"`  // 微信openid
	GiteeUID       string `json:"gitee_uid"`  // gitee uid
	GithubUID      string `json:"github_uid"` // github uid
	// 是否允许主动加好友/创建群聊 0.否 1.是。前端列表行据此显示开关。
	CanInviteOrCreateGroup int `json:"can_invite_or_create_group"`
}

type managerFriendResp struct {
	Name             string `json:"name"`
	UID              string `json:"uid"`
	Remark           string `json:"remark"`
	RelationshipTime string `json:"relationship_time"`
}

type managerDisableUserResp struct {
	Name         string `json:"name"`
	UID          string `json:"uid"`
	ShortNo      string `json:"short_no"`
	Sex          int    `json:"sex"`
	RegisterTime string `json:"register_time"`
	Phone        string `json:"phone"`
	ClosureTime  string `json:"closure_time"`
}

type managerDeviceResp struct {
	ID          int64  `json:"id"`
	DeviceID    string `json:"device_id"`    // 设备ID
	DeviceName  string `json:"device_name"`  // 设备名称
	DeviceModel string `json:"device_model"` // 设备型号
	LastLogin   string `json:"last_login"`   // 设备最后一次登录时间
	Self        int    `json:"self"`         // 是否是本机
}

type userOnlineResp struct {
	UID         string `json:"uid"`
	DeviceFlag  uint8  `json:"device_flag"`
	LastOnline  int    `json:"last_online"`
	LastOffline int    `json:"last_offline"`
	Online      int    `json:"online"`
}

func newUserOnlineResp(m *onlineStatusWeightModel) *userOnlineResp {

	return &userOnlineResp{
		UID:         m.UID,
		DeviceFlag:  m.DeviceFlag,
		LastOnline:  m.LastOnline,
		LastOffline: m.LastOffline,
		Online:      m.Online,
	}
}
