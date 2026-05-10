package user

import (
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

// isBannableIP 判断 IP 是否允许加入黑名单
//
//	不允许：空 / 回环 / 内网 / 链路本地 / 未指定地址
//	防止反向代理未正确配置 X-Forwarded-For 时误把整个内网封死
func isBannableIP(ip string) (ok bool, reason string) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return false, "IP 不能为空"
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false, "IP 格式不正确"
	}
	if parsed.IsLoopback() {
		return false, "不允许封禁回环地址（127.x / ::1）"
	}
	if parsed.IsUnspecified() {
		return false, "不允许封禁未指定地址（0.0.0.0 / ::）"
	}
	if parsed.IsLinkLocalUnicast() || parsed.IsLinkLocalMulticast() {
		return false, "不允许封禁链路本地地址"
	}
	if parsed.IsPrivate() {
		return false, "不允许封禁内网 IP（请检查反向代理是否正确转发 X-Forwarded-For）"
	}
	return true, ""
}

// CMDForceLogout 强制下线 CMD：客户端收到后弹窗显示原因并自动退出登录
const CMDForceLogout = "forceLogout"

// 强制下线匹配类型
const (
	ForceLogoutMatchUser   = "user"
	ForceLogoutMatchIP     = "ip"
	ForceLogoutMatchDevice = "device"
)

// forceLogoutCMDLeadTime 推送 forceLogout CMD 后，留给客户端处理 CMD 的时间
//
//	作用：让 CMD（经 IM 消息通道）先于后续的 QuitUserDevice（经 IM 管理 API）到达客户端，
//	避免 Android 在收到 IM kick 时先于 CMD 弹出「您已在其他设备登录」的错误提示。
//	300ms 是经验值，与 liftBanUser 中实测表现一致。
const forceLogoutCMDLeadTime = 300 * time.Millisecond

// sendForceLogoutCMD 给一批 uid 推送强制下线 CMD，并等待客户端处理 CMD 的时间。
// 客户端 CMDListener 收到 forceLogout 后：
//   - match_type=user：直接退出（已封禁的账号）
//   - match_type=device：客户端比对 device_id 一致才退出（精确）
//   - match_type=ip：直接退出（服务端已筛选「在线且 IP 匹配」）
//
// 调用方在本函数返回后再调用 QuitUserDevice 即可保证 CMD 先到。
func (m *Manager) sendForceLogoutCMD(uids []string, reason, matchType, matchValue string) {
	if len(uids) == 0 {
		return
	}
	const batchSize = 500
	param := map[string]interface{}{
		"reason":      reason,
		"match_type":  matchType,
		"match_value": matchValue,
	}
	sent := false
	for i := 0; i < len(uids); i += batchSize {
		end := i + batchSize
		if end > len(uids) {
			end = len(uids)
		}
		batch := uids[i:end]
		if err := m.ctx.SendCMD(config.MsgCMDReq{
			NoPersist:   true,
			Subscribers: batch,
			CMD:         CMDForceLogout,
			Param:       param,
		}); err != nil {
			m.Error("发送 forceLogout CMD 失败",
				zap.Error(err), zap.String("match_type", matchType), zap.Int("uid_count", len(batch)))
			continue
		}
		sent = true
	}
	// 仅在确实有 CMD 投递成功时才等待，避免无意义的延迟
	if sent {
		time.Sleep(forceLogoutCMDLeadTime)
	}
}

// =========================
// IP 列表 / IP 黑名单
// =========================

// ipList 管理后台 IP 列表（按 (ip, uid) 去重，含关联用户与黑名单状态）
func (m *Manager) ipList(c *wkhttp.Context) {
	if err := c.CheckLoginRoleIsSuperAdmin(); err != nil {
		c.ResponseError(err)
		return
	}
	keyword := strings.TrimSpace(c.Query("keyword"))
	pageIndex, pageSize := c.GetPage()

	rows, err := m.db.queryIPAggList(keyword, uint64(pageSize), uint64(pageIndex))
	if err != nil {
		m.Error("查询 IP 聚合列表失败", zap.Error(err))
		c.ResponseError(errors.New("查询 IP 列表失败"))
		return
	}
	count, err := m.db.queryIPAggCount(keyword)
	if err != nil {
		m.Error("查询 IP 聚合数量失败", zap.Error(err))
		c.ResponseError(errors.New("查询 IP 数量失败"))
		return
	}

	// 批量查询黑名单状态
	ips := make([]string, 0, len(rows))
	for _, r := range rows {
		ips = append(ips, r.LoginIP)
	}
	bls, err := m.ipBlacklistDB.queryByIPs(ips)
	if err != nil {
		m.Error("批量查询 IP 黑名单状态失败", zap.Error(err))
		c.ResponseError(errors.New("查询 IP 黑名单状态失败"))
		return
	}
	bannedSet := map[string]*ipBlacklistModel{}
	for _, b := range bls {
		bannedSet[b.IP] = b
	}

	list := make([]*managerIPListResp, 0, len(rows))
	for _, r := range rows {
		var lastLoginAt string
		if r.LastLoginAt.Valid {
			lastLoginAt = util.ToyyyyMMddHHmm(r.LastLoginAt.Time)
		}
		ban := bannedSet[r.LoginIP]
		var bannedAt, banReason string
		isBanned := 0
		if ban != nil {
			isBanned = 1
			banReason = ban.Reason
			bannedAt = util.ToyyyyMMddHHmm(time.Time(ban.CreatedAt))
		}
		list = append(list, &managerIPListResp{
			LoginIP:       r.LoginIP,
			UID:           r.UID,
			Name:          r.Name,
			Username:      r.Username,
			UserStatus:    r.Status,
			LastLoginTime: lastLoginAt,
			IsBanned:      isBanned,
			BanReason:     banReason,
			BannedAt:      bannedAt,
		})
	}
	c.Response(map[string]interface{}{
		"list":  list,
		"count": count,
	})
}

// ipBlacklistAdd 封禁 IP
func (m *Manager) ipBlacklistAdd(c *wkhttp.Context) {
	if err := c.CheckLoginRoleIsSuperAdmin(); err != nil {
		c.ResponseError(err)
		return
	}
	var req struct {
		IP     string `json:"ip"`
		Reason string `json:"reason"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误"))
		return
	}
	req.IP = strings.TrimSpace(req.IP)
	if ok, reason := isBannableIP(req.IP); !ok {
		c.ResponseError(errors.New(reason))
		return
	}
	if err := m.ipBlacklistDB.upsert(&ipBlacklistModel{
		IP:          req.IP,
		Reason:      req.Reason,
		OperatorUID: c.GetLoginUID(),
	}); err != nil {
		m.Error("封禁 IP 失败", zap.Error(err))
		c.ResponseError(errors.New("封禁 IP 失败"))
		return
	}
	// 异步：通知正在使用此 IP 在线的用户强制下线
	go m.kickByIP(req.IP, req.Reason)
	c.ResponseOK()
}

// kickByIP 强制下线「正在使用此 IP 在线」的用户
//  1. 推送 forceLogout CMD（客户端弹窗+主动退出）
//  2. 调用 IM QuitUserDevice 兜底切断长连
func (m *Manager) kickByIP(ip, reason string) {
	uids, err := m.db.queryOnlineUIDsByLastLoginIP(ip)
	if err != nil {
		m.Error("查询 IP 关联在线用户失败", zap.Error(err), zap.String("ip", ip))
		return
	}
	if len(uids) == 0 {
		return
	}
	tip := reason
	if strings.TrimSpace(tip) == "" {
		tip = "您的登录 IP 已被管理员封禁"
	} else {
		tip = "您的登录 IP 已被管理员封禁。原因：" + tip
	}
	m.sendForceLogoutCMD(uids, tip, ForceLogoutMatchIP, ip)
	for _, uid := range uids {
		if err := m.ctx.QuitUserDevice(uid, -1); err != nil {
			m.Error("强制下线用户失败", zap.Error(err), zap.String("uid", uid))
		}
	}
}

// ipBlacklistRemove 解禁 IP
func (m *Manager) ipBlacklistRemove(c *wkhttp.Context) {
	if err := c.CheckLoginRoleIsSuperAdmin(); err != nil {
		c.ResponseError(err)
		return
	}
	var req struct {
		IP string `json:"ip"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误"))
		return
	}
	req.IP = strings.TrimSpace(req.IP)
	if req.IP == "" {
		c.ResponseError(errors.New("IP 不能为空"))
		return
	}
	if err := m.ipBlacklistDB.delete(req.IP); err != nil {
		m.Error("解禁 IP 失败", zap.Error(err))
		c.ResponseError(errors.New("解禁 IP 失败"))
		return
	}
	c.ResponseOK()
}

// =========================
// 设备列表 / 设备黑名单
// =========================

// deviceList 管理后台设备列表（每条 (device_id, uid) 一行）
func (m *Manager) deviceList(c *wkhttp.Context) {
	if err := c.CheckLoginRoleIsSuperAdmin(); err != nil {
		c.ResponseError(err)
		return
	}
	keyword := strings.TrimSpace(c.Query("keyword"))
	pageIndex, pageSize := c.GetPage()

	rows, err := m.db.queryDeviceAggList(keyword, uint64(pageSize), uint64(pageIndex))
	if err != nil {
		m.Error("查询设备列表失败", zap.Error(err))
		c.ResponseError(errors.New("查询设备列表失败"))
		return
	}
	count, err := m.db.queryDeviceAggCount(keyword)
	if err != nil {
		m.Error("查询设备数量失败", zap.Error(err))
		c.ResponseError(errors.New("查询设备数量失败"))
		return
	}

	deviceIDs := make([]string, 0, len(rows))
	for _, r := range rows {
		deviceIDs = append(deviceIDs, r.DeviceID)
	}
	bls, err := m.deviceBlacklistDB.queryByDeviceIDs(deviceIDs)
	if err != nil {
		m.Error("批量查询设备黑名单状态失败", zap.Error(err))
		c.ResponseError(errors.New("查询设备黑名单状态失败"))
		return
	}
	bannedSet := map[string]*deviceBlacklistModel{}
	for _, b := range bls {
		bannedSet[b.DeviceID] = b
	}

	list := make([]*managerDeviceListResp, 0, len(rows))
	for _, r := range rows {
		ban := bannedSet[r.DeviceID]
		var bannedAt, banReason string
		isBanned := 0
		if ban != nil {
			isBanned = 1
			banReason = ban.Reason
			bannedAt = util.ToyyyyMMddHHmm(time.Time(ban.CreatedAt))
		}
		list = append(list, &managerDeviceListResp{
			ID:            r.Id,
			DeviceID:      r.DeviceID,
			DeviceName:    r.DeviceName,
			DeviceModel:   r.DeviceModel,
			UID:           r.UID,
			Name:          r.Name,
			Username:      r.Username,
			UserStatus:    r.Status,
			LastLoginTime: util.ToyyyyMMddHHmm(time.Unix(r.LastLogin, 0)),
			IsBanned:      isBanned,
			BanReason:     banReason,
			BannedAt:      bannedAt,
		})
	}
	c.Response(map[string]interface{}{
		"list":  list,
		"count": count,
	})
}

// deviceBlacklistAdd 封禁设备
func (m *Manager) deviceBlacklistAdd(c *wkhttp.Context) {
	if err := c.CheckLoginRoleIsSuperAdmin(); err != nil {
		c.ResponseError(err)
		return
	}
	var req struct {
		DeviceID string `json:"device_id"`
		Reason   string `json:"reason"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误"))
		return
	}
	req.DeviceID = strings.TrimSpace(req.DeviceID)
	if req.DeviceID == "" {
		c.ResponseError(errors.New("device_id 不能为空"))
		return
	}
	if err := m.deviceBlacklistDB.upsert(&deviceBlacklistModel{
		DeviceID:    req.DeviceID,
		Reason:      req.Reason,
		OperatorUID: c.GetLoginUID(),
	}); err != nil {
		m.Error("封禁设备失败", zap.Error(err))
		c.ResponseError(errors.New("封禁设备失败"))
		return
	}
	// 异步：通知此 device_id 关联的所有 uid 强制下线
	go m.kickByDevice(req.DeviceID, req.Reason)
	c.ResponseOK()
}

// kickByDevice 强制下线此 device_id 关联的用户
//
//	客户端 CMDListener 收到后，会比对自身 device_id 一致才退出（精确）
//	服务端 QuitUserDevice 是 uid 粒度下线，可能导致同 uid 在其他设备的连接也被断开（可接受：重连即可）
func (m *Manager) kickByDevice(deviceID, reason string) {
	uids, err := m.db.queryUIDsByDeviceID(deviceID)
	if err != nil {
		m.Error("查询设备关联用户失败", zap.Error(err), zap.String("device_id", deviceID))
		return
	}
	if len(uids) == 0 {
		return
	}
	tip := reason
	if strings.TrimSpace(tip) == "" {
		tip = "您的登录设备已被管理员封禁"
	} else {
		tip = "您的登录设备已被管理员封禁。原因：" + tip
	}
	m.sendForceLogoutCMD(uids, tip, ForceLogoutMatchDevice, deviceID)
	// 客户端基于 device_id 自我匹配后退出；不主动 QuitUserDevice，避免同 uid 在其他设备被误踢
}

// deviceBlacklistRemove 解禁设备
func (m *Manager) deviceBlacklistRemove(c *wkhttp.Context) {
	if err := c.CheckLoginRoleIsSuperAdmin(); err != nil {
		c.ResponseError(err)
		return
	}
	var req struct {
		DeviceID string `json:"device_id"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误"))
		return
	}
	req.DeviceID = strings.TrimSpace(req.DeviceID)
	if req.DeviceID == "" {
		c.ResponseError(errors.New("device_id 不能为空"))
		return
	}
	if err := m.deviceBlacklistDB.delete(req.DeviceID); err != nil {
		m.Error("解禁设备失败", zap.Error(err))
		c.ResponseError(errors.New("解禁设备失败"))
		return
	}
	c.ResponseOK()
}

// 防止 strconv 未使用编译错误（保留以便后续扩展分页参数解析）
var _ = strconv.Itoa

// =========================
// resp 结构
// =========================

type managerIPListResp struct {
	LoginIP       string `json:"login_ip"`
	UID           string `json:"uid"`
	Name          string `json:"name"`
	Username      string `json:"username"`
	UserStatus    int    `json:"user_status"` // 用户状态：1 正常 0 封禁
	LastLoginTime string `json:"last_login_time"`
	IsBanned      int    `json:"is_banned"` // 1 已封禁 0 未封禁
	BanReason     string `json:"ban_reason"`
	BannedAt      string `json:"banned_at"`
}

type managerDeviceListResp struct {
	ID            int64  `json:"id"`
	DeviceID      string `json:"device_id"`
	DeviceName    string `json:"device_name"`
	DeviceModel   string `json:"device_model"`
	UID           string `json:"uid"`
	Name          string `json:"name"`
	Username      string `json:"username"`
	UserStatus    int    `json:"user_status"`
	LastLoginTime string `json:"last_login_time"`
	IsBanned      int    `json:"is_banned"`
	BanReason     string `json:"ban_reason"`
	BannedAt      string `json:"banned_at"`
}
