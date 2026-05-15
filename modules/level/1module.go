package level

import (
	"embed"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/base/event"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/model"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/register"
)

//go:embed sql
var sqlFS embed.FS

func init() {
	register.AddModule(func(ctx interface{}) register.Module {
		c := ctx.(*config.Context)
		api := NewAPI(c)

		// 监听用户注册事件：把通过层级邀请码注册的用户挂到对应层级，并加默认好友
		c.AddEventListener(event.EventUserRegister, api.handleUserRegister)

		return register.Module{
			Name: "level",
			SetupAPI: func() register.APIRouter {
				return api
			},
			SQLDir: register.NewSQLFS(sqlFS),
			BussDataSource: register.BussDataSource{
				// 注册侧通过 invite_code 命中层级时返回 invite。
				// invite.Uid 设为层级负责人 uid，使 user 模块识别为有效邀请码。
				// 后续 EventUserRegister 监听器再处理「绑定层级 + 加默认好友」。
				GetInviteCode: func(inviteCode string) (*model.Invite, error) {
					n, err := api.db.queryNodeByInviteCode(inviteCode)
					if err != nil || n == nil {
						return nil, err
					}
					return &model.Invite{
						InviteCode: n.InviteCode,
						Uid:        n.OwnerUID,
						Vercode:    "",
					}, nil
				},
			},
		}
	})
}
