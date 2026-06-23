package common

import (
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	ldb "github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/gocraft/dbr/v2"
)

type appConfigDB struct {
	session *dbr.Session
	ctx     *config.Context
}

func newAppConfigDB(ctx *config.Context) *appConfigDB {

	return &appConfigDB{
		session: ctx.DB(),
		ctx:     ctx,
	}
}

func (a *appConfigDB) query() (*appConfigModel, error) {
	var m *appConfigModel
	_, err := a.session.Select("*").From("app_config").OrderDesc("created_at").Load(&m)
	return m, err
}

func (a *appConfigDB) insert(m *appConfigModel) error {
	_, err := a.session.InsertInto("app_config").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	return err
}
func (a *appConfigDB) updateWithMap(configMap map[string]interface{}, id int64) error {
	_, err := a.session.Update("app_config").SetMap(configMap).Where("id=?", id).Exec()
	return err
}

type appConfigModel struct {
	RSAPrivateKey                  string
	RSAPublicKey                   string
	Version                        int
	SuperToken                     string
	SuperTokenOn                   int
	RevokeSecond                   int    // 消息可撤回时长
	WelcomeMessage                 string // 登录欢迎语
	NewUserJoinSystemGroup         int    // 新用户是否加入系统群聊
	SearchByPhone                  int    // 是否可通过手机号搜索
	RegisterInviteOn               int    // 开启注册邀请机制
	SendWelcomeMessageOn           int    // 开启注册登录发送欢迎语
	InviteSystemAccountJoinGroupOn int    // 开启系统账号加入群聊
	RegisterUserMustCompleteInfoOn int    // 注册用户是否必须完善个人信息
	ChannelPinnedMessageMaxCount   int    // 频道置顶消息最大数量
	CanModifyApiUrl                int    // 是否可以修改API地址
	DisableGroupMessageOn          int    // 是否开启群聊禁言（含群发消息/建群/加群成员/加好友）
	DisablePrivateMessageOn        int    // 是否开启私聊禁言（禁止私聊发消息）
	MuteTextOfGroup                string // 群聊禁言客户端展示文案
	MuteTextOfPrivate              string // 私聊禁言客户端展示文案
	SmsVerifyOn                    int    // 是否开启短信验证码
	ldb.BaseModel
}
