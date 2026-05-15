package level

import (
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/common"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"go.uber.org/zap"
)

// handleUserRegister 监听用户注册事件，处理「层级邀请码注册」：
//  1. 把 user.level_node_no 设置为对应层级编号
//  2. 把新用户与所有默认好友互为好友（IM 白名单同步）
//
// 与 user 模块自身的 handleUserRegister 监听器并行存在；user 模块只负责把
// 新用户与「邀请人（即层级负责人）」互为好友，本监听器只负责追加默认好友。
func (a *API) handleUserRegister(data []byte, commit config.EventCommit) {
	var req map[string]interface{}
	if err := util.ReadJsonByByte(data, &req); err != nil {
		a.Error("level 监听用户注册：解析参数失败", zap.Error(err))
		commit(err)
		return
	}
	uid, _ := req["uid"].(string)
	inviteCode, _ := req["invite_code"].(string)
	if uid == "" || inviteCode == "" {
		commit(nil)
		return
	}
	node, err := a.db.queryNodeByInviteCode(inviteCode)
	if err != nil {
		a.Error("level 查邀请码失败", zap.Error(err))
		commit(nil) // 不阻断主流程
		return
	}
	if node == nil {
		commit(nil)
		return
	}
	// 1) 绑定层级
	if err := a.db.updateUserLevelNode(uid, node.NodeNo); err != nil {
		a.Error("level 设置用户层级失败", zap.Error(err), zap.String("uid", uid), zap.String("node_no", node.NodeNo))
		// 不 return，仍尽力加默认好友
	}
	// 2) 加默认好友
	friends, err := a.db.queryDefaultFriends(node.NodeNo)
	if err != nil {
		a.Error("level 查默认好友失败", zap.Error(err))
		commit(nil)
		return
	}
	if len(friends) == 0 {
		commit(nil)
		return
	}
	tx, err := a.ctx.DB().Begin()
	if err != nil {
		a.Error("level 默认好友事务开启失败", zap.Error(err))
		commit(nil)
		return
	}
	defer func() {
		if e := recover(); e != nil {
			tx.Rollback()
			panic(e)
		}
	}()
	for _, fuid := range friends {
		if fuid == "" || fuid == uid {
			continue
		}
		if err := a.db.addFriendBidirectional(tx, uid, fuid); err != nil {
			a.Error("level 添加默认好友失败", zap.Error(err))
			tx.Rollback()
			commit(nil)
			return
		}
		// IM 白名单
		if err := a.ctx.IMWhitelistAdd(config.ChannelWhitelistReq{
			ChannelReq: config.ChannelReq{
				ChannelID:   uid,
				ChannelType: common.ChannelTypePerson.Uint8(),
			},
			UIDs: []string{fuid},
		}); err != nil {
			a.Warn("level 添加IM白名单失败", zap.Error(err))
		}
		if err := a.ctx.IMWhitelistAdd(config.ChannelWhitelistReq{
			ChannelReq: config.ChannelReq{
				ChannelID:   fuid,
				ChannelType: common.ChannelTypePerson.Uint8(),
			},
			UIDs: []string{uid},
		}); err != nil {
			a.Warn("level 添加IM白名单失败", zap.Error(err))
		}
	}
	if err := tx.Commit(); err != nil {
		a.Error("level 默认好友事务提交失败", zap.Error(err))
		tx.Rollback()
	}
	commit(nil)
}
