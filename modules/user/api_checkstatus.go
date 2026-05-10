package user

import (
	"strings"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/common"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

// CheckStatus 匹配类型（与 forceLogout CMD 的 match_type 语义一致，便于客户端复用处理逻辑）
const (
	CheckStatusMatchUser   = "user"
	CheckStatusMatchIP     = "ip"
	CheckStatusMatchDevice = "device"
)

// checkStatusResp 返回当前登录态是否已被风控拦截
type checkStatusResp struct {
	Banned    bool   `json:"banned"`               // 是否被封禁；true 时客户端应弹窗并 logout
	MatchType string `json:"match_type,omitempty"` // 命中的维度：user / ip / device
	Reason    string `json:"reason,omitempty"`     // 面向用户的提示文案
}

// checkStatus 客户端启动或从后台恢复时调用
//
// 用途：
//   - 登录态下，如果账号 / 当前 IP / 当前 device_id 之一被管理员加入黑名单，
//     而客户端之前没收到 forceLogout CMD（如进程被杀、后台长期休眠），
//     此接口给客户端一个主动发现的时机，让客户端自行弹窗并退出登录。
//
// 请求参数（均 optional）：
//
//	GET /v1/user/checkstatus?device_id=xxx
//	Header: token: <token>
//
// 检查顺序：user -> ip -> device，优先返回最先命中的原因。
func (u *User) checkStatus(c *wkhttp.Context) {
	loginUID := c.MustGet("uid").(string)

	// 1) 用户是否被封禁
	userInfo, err := u.db.QueryByUID(loginUID)
	if err != nil {
		u.Error("查询用户信息失败", zap.Error(err), zap.String("uid", loginUID))
		c.ResponseError(errors.New("查询用户状态失败"))
		return
	}
	if userInfo == nil {
		// 账号被删除等异常情况，按封禁处理，让客户端退登
		c.Response(checkStatusResp{
			Banned:    true,
			MatchType: CheckStatusMatchUser,
			Reason:    "账号不存在或已被注销",
		})
		return
	}
	if userInfo.Status == int(common.UserDisable) {
		c.Response(checkStatusResp{
			Banned:    true,
			MatchType: CheckStatusMatchUser,
			Reason:    "您的账号已被管理员封禁",
		})
		return
	}

	// 2) 当前请求 IP 是否在黑名单
	publicIP := util.GetClientPublicIP(c.Request)
	if publicIP != "" {
		banned, err := u.ipBlacklistDB.exist(publicIP)
		if err != nil {
			u.Error("查询 IP 黑名单失败", zap.Error(err), zap.String("ip", publicIP))
			c.ResponseError(errors.New("查询风控状态失败"))
			return
		}
		if banned {
			c.Response(checkStatusResp{
				Banned:    true,
				MatchType: CheckStatusMatchIP,
				Reason:    "您当前的登录 IP 已被管理员封禁",
			})
			return
		}
	}

	// 3) 当前 device_id 是否在黑名单（客户端传递）
	deviceID := strings.TrimSpace(c.Query("device_id"))
	if deviceID != "" {
		banned, err := u.deviceBlacklistDB.exist(deviceID)
		if err != nil {
			u.Error("查询设备黑名单失败", zap.Error(err), zap.String("device_id", deviceID))
			c.ResponseError(errors.New("查询风控状态失败"))
			return
		}
		if banned {
			c.Response(checkStatusResp{
				Banned:    true,
				MatchType: CheckStatusMatchDevice,
				Reason:    "您当前的登录设备已被管理员封禁",
			})
			return
		}
	}

	c.Response(checkStatusResp{Banned: false})
}
