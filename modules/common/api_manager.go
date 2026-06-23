package common

import (
	"errors"
	"strings"
	"time"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/log"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"go.uber.org/zap"
)

// CMDAppConfigUpdate 当管理后台修改 app 全局配置（如禁言开关）时下发的 CMD
// 客户端收到后立即重新拉取 /v1/common/appconfig，避免轮询延迟
const CMDAppConfigUpdate = "appconfigUpdate"

// Manager 通用后台管理api
type Manager struct {
	ctx *config.Context
	log.Log
	db          *db
	appconfigDB *appConfigDB
}

// NewManager NewManager
func NewManager(ctx *config.Context) *Manager {
	return &Manager{
		ctx:         ctx,
		Log:         log.NewTLog("commonManager"),
		db:          newDB(ctx.DB()),
		appconfigDB: newAppConfigDB(ctx),
	}
}

// Route 配置路由规则
func (m *Manager) Route(r *wkhttp.WKHttp) {
	auth := r.Group("/v1/manager", m.ctx.AuthMiddleware(r))
	{
		auth.GET("/common/appconfig", m.appconfig)                // 获取app配置
		auth.POST("/common/appconfig", m.updateConfig)            // 修改app配置
		auth.GET("/common/appmodule", m.getAppModule)             // 获取app模块
		auth.PUT("/common/appmodule", m.updateAppModule)          // 修改app模块
		auth.POST("/common/appmodule", m.addAppModule)            // 新增app模块
		auth.DELETE("/common/:sid/appmodule", m.deleteAppModule)  // 删除app模块
		auth.GET("/common/secure_channel", m.getSecureChannel)    // 获取加密通道配置
		auth.PUT("/common/secure_channel", m.updateSecureChannel) // 更新加密通道配置
	}
}
func (m *Manager) deleteAppModule(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}

	sid := c.Param("sid")
	if strings.TrimSpace(sid) == "" {
		c.ResponseError(errors.New("sid不能为空！"))
		return
	}
	module, err := m.db.queryAppModuleWithSid(sid)
	if err != nil {
		m.Error("查询app模块错误", zap.Error(err))
		c.ResponseError(errors.New("查询app模块错误"))
		return
	}
	if module == nil {
		c.ResponseError(errors.New("删除的模块不存在"))
		return
	}
	err = m.db.deleteAppModule(sid)
	if err != nil {
		m.Error("删除app模块错误", zap.Error(err))
		c.ResponseError(errors.New("删除app模块错误"))
		return
	}
	c.ResponseOK()
}

// 新增app模块
func (m *Manager) addAppModule(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type ReqVO struct {
		SID    string `json:"sid"`
		Name   string `json:"name"`
		Desc   string `json:"desc"`
		Status int    `json:"status"`
	}
	var req ReqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}

	if strings.TrimSpace(req.SID) == "" || strings.TrimSpace(req.Desc) == "" || strings.TrimSpace(req.Name) == "" {
		c.ResponseError(errors.New("名称/ID/介绍不能为空！"))
		return
	}
	module, err := m.db.queryAppModuleWithSid(req.SID)
	if err != nil {
		m.Error("查询app模块错误", zap.Error(err))
		c.ResponseError(errors.New("查询app模块错误"))
		return
	}
	if module != nil && module.SID != "" {
		c.ResponseError(errors.New("该sid模块已存在"))
		return
	}
	_, err = m.db.insertAppModule(&appModuleModel{
		SID:    req.SID,
		Name:   req.Name,
		Desc:   req.Desc,
		Status: req.Status,
	})
	if err != nil {
		m.Error("新增app模块错误", zap.Error(err))
		c.ResponseError(errors.New("新增app模块错误"))
		return
	}
	c.ResponseOK()
}
func (m *Manager) updateAppModule(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type ReqVO struct {
		SID    string `json:"sid"`
		Name   string `json:"name"`
		Desc   string `json:"desc"`
		Status int    `json:"status"`
	}
	var req ReqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}

	if strings.TrimSpace(req.SID) == "" || strings.TrimSpace(req.Desc) == "" || strings.TrimSpace(req.Name) == "" {
		c.ResponseError(errors.New("名称/ID/介绍不能为空！"))
		return
	}
	module, err := m.db.queryAppModuleWithSid(req.SID)
	if err != nil {
		m.Error("查询app模块错误", zap.Error(err))
		c.ResponseError(errors.New("查询app模块错误"))
		return
	}
	if module == nil {
		c.ResponseError(errors.New("不存在该模块"))
		return
	}
	module.Name = req.Name
	module.Desc = req.Desc
	module.Status = req.Status
	err = m.db.updateAppModule(module)
	if err != nil {
		m.Error("修改app模块错误", zap.Error(err))
		c.ResponseError(errors.New("修改app模块错误"))
		return
	}
	c.ResponseOK()
}

// 获取app模块
func (m *Manager) getAppModule(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	modules, err := m.db.queryAppModule()
	if err != nil {
		m.Error("查询app模块错误", zap.Error(err))
		c.ResponseError(errors.New("查询app模块错误"))
		return
	}
	list := make([]*managerAppModule, 0)
	if len(modules) > 0 {
		for _, module := range modules {
			list = append(list, &managerAppModule{
				SID:    module.SID,
				Name:   module.Name,
				Desc:   module.Desc,
				Status: module.Status,
			})
		}
	}
	c.Response(list)
}
func (m *Manager) updateConfig(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type reqVO struct {
		RevokeSecond                   int    `json:"revoke_second"`
		WelcomeMessage                 string `json:"welcome_message"`
		NewUserJoinSystemGroup         int    `json:"new_user_join_system_group"`
		SearchByPhone                  int    `json:"search_by_phone"`
		RegisterInviteOn               int    `json:"register_invite_on"`                  // 开启注册邀请机制
		SendWelcomeMessageOn           int    `json:"send_welcome_message_on"`             // 开启注册登录发送欢迎语
		InviteSystemAccountJoinGroupOn int    `json:"invite_system_account_join_group_on"` // 开启系统账号加入群聊
		RegisterUserMustCompleteInfoOn int    `json:"register_user_must_complete_info_on"` // 注册用户必须填写完整信息
		ChannelPinnedMessageMaxCount   int    `json:"channel_pinned_message_max_count"`    // 频道置顶消息最大数量
		CanModifyApiUrl                int    `json:"can_modify_api_url"`                  // 是否可以修改api地址
		DisableGroupMessageOn          int    `json:"disable_group_message_on"`            // 群聊禁言开关
		DisablePrivateMessageOn        int    `json:"disable_private_message_on"`          // 私聊禁言开关
		MuteTextOfGroup                string `json:"mute_text_of_group"`                  // 群聊禁言文案
		MuteTextOfPrivate              string `json:"mute_text_of_private"`                // 私聊禁言文案
		SmsVerifyOn                    int    `json:"sms_verify_on"`                       // 是否开启短信验证码
	}
	var req reqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	appConfigM, err := m.appconfigDB.query()
	if err != nil {
		m.Error("查询应用配置失败！", zap.Error(err))
		c.ResponseError(errors.New("查询应用配置失败！"))
		return
	}
	configMap := map[string]interface{}{}
	configMap["revoke_second"] = req.RevokeSecond
	configMap["welcome_message"] = req.WelcomeMessage
	configMap["new_user_join_system_group"] = req.NewUserJoinSystemGroup
	configMap["search_by_phone"] = req.SearchByPhone
	configMap["register_invite_on"] = req.RegisterInviteOn
	configMap["send_welcome_message_on"] = req.SendWelcomeMessageOn
	configMap["invite_system_account_join_group_on"] = req.InviteSystemAccountJoinGroupOn
	configMap["register_user_must_complete_info_on"] = req.RegisterUserMustCompleteInfoOn
	configMap["channel_pinned_message_max_count"] = req.ChannelPinnedMessageMaxCount
	configMap["can_modify_api_url"] = req.CanModifyApiUrl
	configMap["disable_group_message_on"] = req.DisableGroupMessageOn
	configMap["disable_private_message_on"] = req.DisablePrivateMessageOn
	configMap["mute_text_of_group"] = req.MuteTextOfGroup
	configMap["mute_text_of_private"] = req.MuteTextOfPrivate
	configMap["sms_verify_on"] = req.SmsVerifyOn
	err = m.appconfigDB.updateWithMap(configMap, appConfigM.Id)
	if err != nil {
		m.Error("修改app配置信息错误", zap.Error(err))
		c.ResponseError(errors.New("修改app配置信息错误"))
		return
	}
	// 异步广播 CMD：让所有在线客户端立即重新拉取最新配置（无需等待轮询）
	go m.broadcastAppConfigUpdate()
	c.ResponseOK()
}

// broadcastAppConfigUpdate 给所有用户分批下发 appconfigUpdate CMD
// 客户端收到该 CMD 后会重新拉取 /v1/common/appconfig 同步最新值
func (m *Manager) broadcastAppConfigUpdate() {
	type uidRow struct {
		UID string `db:"uid"`
	}
	var rows []*uidRow
	_, err := m.ctx.DB().Select("uid").From("user").Where("is_destroy=0 and bench_no=''").Load(&rows)
	if err != nil {
		m.Error("broadcastAppConfigUpdate: 查询用户列表失败", zap.Error(err))
		return
	}
	if len(rows) == 0 {
		return
	}
	const batchSize = 1000
	batch := make([]string, 0, batchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		err := m.ctx.SendCMD(config.MsgCMDReq{
			NoPersist:   true,
			Subscribers: batch,
			CMD:         CMDAppConfigUpdate,
		})
		if err != nil {
			m.Error("broadcastAppConfigUpdate: 发送 CMD 失败", zap.Error(err))
		}
		batch = batch[:0]
	}
	for i, r := range rows {
		batch = append(batch, r.UID)
		if len(batch) >= batchSize {
			flush()
			// 分批之间休眠，避免压力过大
			time.Sleep(time.Second)
		}
		_ = i
	}
	flush()
}
func (m *Manager) appconfig(c *wkhttp.Context) {
	err := c.CheckLoginRole()
	if err != nil {
		c.ResponseError(err)
		return
	}
	appconfig, err := m.appconfigDB.query()
	if err != nil {
		m.Error("查询应用配置失败！", zap.Error(err))
		c.ResponseError(errors.New("查询应用配置失败！"))
		return
	}
	var revokeSecond = 0
	var newUserJoinSystemGroup = 1
	var welcomeMessage = ""
	var searchByPhone = 1
	var registerInviteOn = 0
	var sendWelcomeMessageOn = 0
	var inviteSystemAccountJoinGroupOn = 0
	var registerUserMustCompleteInfoOn = 0
	var channelPinnedMessageMaxCount = 10
	var canModifyApiUrl = 0
	var disableGroupMessageOn = 0
	var disablePrivateMessageOn = 0
	var muteTextOfGroup = ""
	var muteTextOfPrivate = ""
	var smsVerifyOn = 1
	if appconfig != nil {
		revokeSecond = appconfig.RevokeSecond
		welcomeMessage = appconfig.WelcomeMessage
		newUserJoinSystemGroup = appconfig.NewUserJoinSystemGroup
		searchByPhone = appconfig.SearchByPhone
		registerInviteOn = appconfig.RegisterInviteOn
		sendWelcomeMessageOn = appconfig.SendWelcomeMessageOn
		inviteSystemAccountJoinGroupOn = appconfig.InviteSystemAccountJoinGroupOn
		registerUserMustCompleteInfoOn = appconfig.RegisterUserMustCompleteInfoOn
		channelPinnedMessageMaxCount = appconfig.ChannelPinnedMessageMaxCount
		canModifyApiUrl = appconfig.CanModifyApiUrl
		disableGroupMessageOn = appconfig.DisableGroupMessageOn
		disablePrivateMessageOn = appconfig.DisablePrivateMessageOn
		muteTextOfGroup = appconfig.MuteTextOfGroup
		muteTextOfPrivate = appconfig.MuteTextOfPrivate
		smsVerifyOn = appconfig.SmsVerifyOn
	}
	if revokeSecond == 0 {
		revokeSecond = 120
	}
	if welcomeMessage == "" {
		welcomeMessage = m.ctx.GetConfig().WelcomeMessage
	}
	c.Response(&managerAppConfigResp{
		RevokeSecond:                   revokeSecond,
		WelcomeMessage:                 welcomeMessage,
		NewUserJoinSystemGroup:         newUserJoinSystemGroup,
		SearchByPhone:                  searchByPhone,
		RegisterInviteOn:               registerInviteOn,
		SendWelcomeMessageOn:           sendWelcomeMessageOn,
		InviteSystemAccountJoinGroupOn: inviteSystemAccountJoinGroupOn,
		RegisterUserMustCompleteInfoOn: registerUserMustCompleteInfoOn,
		ChannelPinnedMessageMaxCount:   channelPinnedMessageMaxCount,
		CanModifyApiUrl:                canModifyApiUrl,
		DisableGroupMessageOn:          disableGroupMessageOn,
		DisablePrivateMessageOn:        disablePrivateMessageOn,
		MuteTextOfGroup:                muteTextOfGroup,
		MuteTextOfPrivate:              muteTextOfPrivate,
		SmsVerifyOn:                    smsVerifyOn,
	})
}

type managerAppConfigResp struct {
	RevokeSecond                   int    `json:"revoke_second"`
	WelcomeMessage                 string `json:"welcome_message"`
	NewUserJoinSystemGroup         int    `json:"new_user_join_system_group"`
	SearchByPhone                  int    `json:"search_by_phone"`
	RegisterInviteOn               int    `json:"register_invite_on"`                  // 开启注册邀请机制
	SendWelcomeMessageOn           int    `json:"send_welcome_message_on"`             // 开启注册登录发送欢迎语
	InviteSystemAccountJoinGroupOn int    `json:"invite_system_account_join_group_on"` // 开启系统账号加入群聊
	RegisterUserMustCompleteInfoOn int    `json:"register_user_must_complete_info_on"` // 注册用户必须填写完整信息
	ChannelPinnedMessageMaxCount   int    `json:"channel_pinned_message_max_count"`    // 频道置顶消息最大数量
	CanModifyApiUrl                int    `json:"can_modify_api_url"`                  // 是否可以修改api地址
	DisableGroupMessageOn          int    `json:"disable_group_message_on"`            // 群聊禁言开关
	DisablePrivateMessageOn        int    `json:"disable_private_message_on"`          // 私聊禁言开关
	MuteTextOfGroup                string `json:"mute_text_of_group"`                  // 群聊禁言文案
	MuteTextOfPrivate              string `json:"mute_text_of_private"`                // 私聊禁言文案
	SmsVerifyOn                    int    `json:"sms_verify_on"`                       // 是否开启短信验证码
}

type managerAppModule struct {
	SID    string `json:"sid"`
	Name   string `json:"name"`
	Desc   string `json:"desc"`
	Status int    `json:"status"` // 模块状态 1.可选 0.不可选 2.选中不可编辑
}
