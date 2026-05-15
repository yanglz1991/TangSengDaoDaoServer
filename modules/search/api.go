package search

import (
	"errors"
	"fmt"
	"strings"

	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/group"
	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/message"
	"github.com/TangSengDaoDao/TangSengDaoDaoServer/modules/user"
	"github.com/TangSengDaoDao/TangSengDaoDaoServer/pkg/log"
	"github.com/TangSengDaoDao/TangSengDaoDaoServer/pkg/util"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/common"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"go.uber.org/zap"
)

type Search struct {
	ctx *config.Context
	log.Log
	userService    user.IService
	groupService   group.IService
	messageService message.IService
}

func New(ctx *config.Context) *Search {
	s := &Search{
		ctx:            ctx,
		Log:            log.NewTLog("search"),
		userService:    user.NewService(ctx),
		groupService:   group.NewService(ctx),
		messageService: message.NewService(ctx),
	}
	return s
}

func (s *Search) Route(r *wkhttp.WKHttp) {
	searchs := r.Group("/v1/search", s.ctx.AuthMiddleware(r))
	{
		searchs.POST("/global", s.global) // 全局搜索
	}
}

func (s *Search) global(c *wkhttp.Context) {
	loginUID := c.GetLoginUID()
	var req struct {
		OnlyMessage int    `json:"only_message"` // 只加载消息
		ContentType []int  `json:"content_type"` // 消息类型
		Keyword     string `json:"keyword"`      // 搜索关键字
		FromUID     string `json:"from_uid"`     // 发送者uid
		ChannelID   string `json:"channel_id"`   // 频道ID
		ChannelType uint8  `json:"channel_type"` // 频道类型
		Topic       string `json:"topic"`        // 根据topic搜索
		Limit       int    `json:"limit"`        // 查询限制数量
		Page        int    `json:"page"`         // 页码，分页使用，默认为1
		StartTime   int64  `json:"start_time"`   //  消息时间（开始）
		EndTime     int64  `json:"end_time"`     // 消息时间（结束，结果不包含end_time）
	}
	if err := c.BindJSON(&req); err != nil {
		s.Error("数据格式有误！", zap.Error(err))
		c.ResponseError(errors.New("数据格式有误！"))
		return
	}
	if req.Limit > 100 {
		req.Limit = 100
	}
	payload := map[string]interface{}{
		"content": req.Keyword,
		"name":    req.Keyword,
	}
	highlights := []string{"payload.content", "payload.name"}

	// 查询消息
	// 注意：仅当存在 keyword 或 only_message=1 / 指定了频道/topic 等"必须查消息"的场景才调用
	// WuKongIM 的全文搜索插件（wk.plugin.search/usersearch）。
	// 部分部署未启用该插件，调用会失败；为避免因消息搜索失败导致整个全局搜索（包括好友/群）
	// 返回错误，这里在失败时降级为消息列表为空，仍正常返回好友/群搜索结果。
	var msgResp *config.SearchUserMessageResp
	needSearchMessages := req.OnlyMessage == 1 ||
		req.ChannelID != "" ||
		req.Topic != "" ||
		req.FromUID != "" ||
		strings.TrimSpace(req.Keyword) != ""
	if needSearchMessages {
		var msgErr error
		msgResp, msgErr = s.ctx.IMSearchUserMessages(&config.SearchUserMessageReq{
			UID:          loginUID,
			Payload:      payload,
			PayloadTypes: req.ContentType,
			Limit:        req.Limit,
			Page:         req.Page,
			FromUID:      req.FromUID,
			ChannelID:    req.ChannelID,
			ChannelType:  req.ChannelType,
			Topic:        req.Topic,
			StartTime:    req.StartTime,
			EndTime:      req.EndTime,
			Highlights:   highlights,
		})
		if msgErr != nil {
			// 仅当客户端明确"只搜消息"时（only_message=1，例如频道内聊天/文件 tab）才报错，
			// 否则降级为消息列表为空，保留好友/群搜索结果，避免首页全局搜索"什么都没有"。
			if req.OnlyMessage == 1 {
				s.Error("查询悟空IM消息错误", zap.Error(msgErr))
				c.ResponseError(errors.New("查询悟空IM消息错误"))
				return
			}
			s.Warn("查询悟空IM消息错误，降级为仅返回好友/群搜索结果", zap.Error(msgErr))
			msgResp = nil
		}
	}
	channelIds := make([]string, 0)
	messageIds := make([]string, 0)
	if msgResp != nil && len(msgResp.Messages) > 0 {
		for _, m := range msgResp.Messages {
			messageIds = append(messageIds, m.MessageIDStr)
			channelIds = append(channelIds, m.ChannelID)
		}
	}
	// 查询撤回标记
	revokedMsgExtras, err := s.messageService.GetRevokedMessages(messageIds)
	if err != nil {
		s.Error("查询消息撤回消息错误", zap.Error(err))
		c.ResponseError(errors.New("查询消息撤回消息错误"))
		return
	}
	// 查询后台管理删除标记
	deletedMsgExtras, err := s.messageService.GetDeletedMessages(messageIds)
	if err != nil {
		s.Error("查询消息删除消息错误", zap.Error(err))
		c.ResponseError(errors.New("查询消息删除消息错误"))
		return
	}
	// 查询登录用户的删除标记
	deletedMsgUserExtras, err := s.messageService.GetDeletedMessagesWithUID(loginUID, messageIds)
	if err != nil {
		s.Error("查询消息删除消息错误", zap.Error(err))
		c.ResponseError(errors.New("查询消息删除消息错误"))
		return
	}

	// 查询登录用户清空channel消息标记
	channelOffsetResps, err := s.messageService.GetChannelOffsetWithUID(loginUID, channelIds)
	if err != nil {
		s.Error("查询用户清空channel消息标记错误", zap.Error(err))
		c.ResponseError(errors.New("查询用户清空channel消息标记错误"))
		return
	}

	// 1. 预处理：构建 Map（O(n) 一次性处理）
	revokedMap := make(map[string]bool, len(revokedMsgExtras))
	for _, extra := range revokedMsgExtras {
		revokedMap[extra.MessageIDStr] = true
	}

	deletedMap := make(map[string]bool, len(deletedMsgExtras))
	for _, extra := range deletedMsgExtras {
		if extra.IsMutualDeleted == 1 {
			deletedMap[extra.MessageIDStr] = true
		}
	}

	deletedUserMap := make(map[string]bool, len(deletedMsgUserExtras))
	for _, extra := range deletedMsgUserExtras {
		if extra.MessageIsDeleted == 1 {
			deletedUserMap[extra.MessageIDStr] = true
		}
	}

	// channelID -> 清空到的 messageSeq
	channelOffsetMap := make(map[string]uint32, len(channelOffsetResps))
	for _, offset := range channelOffsetResps {
		channelOffsetMap[offset.ChannelID] = offset.MessageSeq
	}

	realMessages := make([]*config.MessageResp, 0)
	if msgResp != nil && len(msgResp.Messages) > 0 {
		for _, m := range msgResp.Messages {
			// O(1) 检查是否撤回
			if revokedMap[m.MessageIDStr] {
				continue
			}

			// O(1) 检查是否后台删除
			if deletedMap[m.MessageIDStr] {
				continue
			}

			// O(1) 检查是否用户删除
			if deletedUserMap[m.MessageIDStr] {
				continue
			}

			// O(1) 检查是否清空channel消息
			if offsetSeq, ok := channelOffsetMap[m.ChannelID]; ok && offsetSeq >= m.MessageSeq {
				continue
			}

			realMessages = append(realMessages, m)
		}
	}
	groupIds := make([]string, 0)
	uids := make([]string, 0)
	msgFromUids := make([]string, 0)

	if len(realMessages) > 0 {
		for _, m := range realMessages {
			if m.ChannelType == common.ChannelTypeGroup.Uint8() {
				groupIds = append(groupIds, m.ChannelID)
			} else if m.ChannelType == common.ChannelTypePerson.Uint8() {
				uids = append(uids, m.ChannelID)
			}
			if m.FromUID != "" {
				msgFromUids = append(msgFromUids, m.FromUID)
			}
		}
	}

	// 是否需要按关键字搜索好友/群（仅在用户输入了非空关键字时才搜索；
	// 否则像 keyword="" 这种"初始打开搜索面板"的请求会因为 strings.Contains(name, "") 恒为 true
	// 把全部加入的群/全部好友都返回，体验异常）。
	keywordTrimmed := strings.TrimSpace(req.Keyword)
	searchFriendsAndGroups := req.OnlyMessage == 0 && keywordTrimmed != ""

	var joinedGroups []*group.InfoResp
	if searchFriendsAndGroups {
		joinedGroups, err = s.groupService.GetGroupsWithMemberUID(loginUID)
		if err != nil {
			s.Error("查询加入的群列表错误", zap.Error(err))
			c.ResponseError(errors.New("查询加入的群列表错误"))
			return
		}
		if len(joinedGroups) > 0 {
			for _, group := range joinedGroups {
				groupIds = append(groupIds, group.GroupNo)
			}
		}
	}

	var groups []*group.GroupResp
	var users []*user.UserDetailResp
	if len(groupIds) > 0 {
		groups, err = s.groupService.GetGroupDetails(groupIds, loginUID)
		if err != nil {
			s.Error("查询群列表错误", zap.Error(err))
			c.ResponseError(errors.New("查询群列表错误"))
			return
		}
	}
	if len(msgFromUids) > 0 {
		uids = append(uids, msgFromUids...)
	}
	if len(uids) > 0 {
		realUids := util.RemoveRepeatedElement(uids)
		users, err = s.userService.GetUserDetails(realUids, loginUID)
		if err != nil {
			s.Error("查询用户列表错误", zap.Error(err))
			c.ResponseError(errors.New("查询用户列表错误"))
			return
		}
	}

	// 加入的群
	groupResps := make([]*channelResp, 0)
	if searchFriendsAndGroups && len(joinedGroups) > 0 {
		for _, g := range joinedGroups {
			isAdd := false
			remark := ""
			if strings.Contains(g.Name, keywordTrimmed) {
				isAdd = true
			}
			if len(groups) > 0 {
				for _, group := range groups {
					if group.GroupNo == g.GroupNo {
						remark = group.Remark
						if strings.Contains(group.Remark, keywordTrimmed) {
							isAdd = true
						}
						break
					}
				}
			}
			if isAdd {
				name := strings.ReplaceAll(g.Name, keywordTrimmed, fmt.Sprintf("<mark>%s</mark>", keywordTrimmed))
				groupResps = append(groupResps, &channelResp{
					ChannelID:     g.GroupNo,
					ChannelType:   common.ChannelTypeGroup.Uint8(),
					ChannelName:   name,
					ChannelRemark: remark,
				})
			}
		}
	}

	// 查询好友
	friendResps := make([]*channelResp, 0)
	if searchFriendsAndGroups {
		friends, err := s.userService.SearchFriendsWithKeyword(loginUID, keywordTrimmed)
		if err != nil {
			s.Error("查询好友错误", zap.Error(err))
			c.ResponseError(err)
			return
		}
		if len(friends) > 0 {
			for _, friend := range friends {
				// 同时支持昵称与备注模糊匹配：
				//  - 备注命中时优先用备注作为展示文案（与三端"备注优先于昵称"的列表惯例一致）；
				//  - 否则使用昵称。命中字段做 <mark> 高亮，未命中字段保持原样。
				nameMatch := strings.Contains(friend.Name, keywordTrimmed)
				remarkMatch := friend.Remark != "" && strings.Contains(friend.Remark, keywordTrimmed)
				highlight := func(s string) string {
					if s == "" || keywordTrimmed == "" {
						return s
					}
					return strings.ReplaceAll(s, keywordTrimmed, fmt.Sprintf("<mark>%s</mark>", keywordTrimmed))
				}
				var displayName string
				switch {
				case remarkMatch:
					displayName = highlight(friend.Remark)
				case nameMatch:
					displayName = highlight(friend.Name)
				default:
					// 走到这里说明 SQL 命中了行（极少见，例如大小写敏感/排序规则差异），
					// 兜底优先展示备注，没有备注则展示昵称，并尽量做一次高亮。
					if friend.Remark != "" {
						displayName = highlight(friend.Remark)
					} else {
						displayName = highlight(friend.Name)
					}
				}
				friendResps = append(friendResps, &channelResp{
					ChannelID:     friend.UID,
					ChannelName:   displayName,
					ChannelType:   common.ChannelTypePerson.Uint8(),
					ChannelRemark: friend.Remark,
				})
			}
		}
	}

	messagesResp := make([]*messageResp, 0)
	if len(realMessages) > 0 {
		for _, msg := range realMessages {
			var isDeleted int8 = 0
			setting := config.SettingFromUint8(msg.Setting)
			var payloadMap map[string]interface{}
			if setting.Signal {
				payloadMap = map[string]interface{}{
					"type": common.SignalError.Int(),
				}
			} else {
				err := util.ReadJsonByByte(msg.Payload, &payloadMap)
				if err != nil {
					log.Warn("负荷数据不是json格式！", zap.Error(err), zap.String("payload", string(msg.Payload)))
				}
				if len(payloadMap) > 0 {
					visibles := payloadMap["visibles"]
					if visibles != nil {
						visiblesArray := visibles.([]interface{})
						if len(visiblesArray) > 0 {
							isDeleted = 1
							for _, limitUID := range visiblesArray {
								if limitUID == loginUID {
									isDeleted = 0
								}
							}
						}
					}
				} else {
					payloadMap = map[string]interface{}{
						"type": common.ContentError.Int(),
					}
				}
			}
			if isDeleted == 1 {
				continue
			}
			var tempChannel *channelResp
			if msg.ChannelType == common.ChannelTypePerson.Uint8() {
				for _, user := range users {
					if user.UID == msg.ChannelID {
						tempChannel = &channelResp{
							ChannelID:     user.UID,
							ChannelType:   common.ChannelTypePerson.Uint8(),
							ChannelRemark: user.Remark,
							ChannelName:   user.Name,
						}
						break
					}
				}
			}
			var fromChannel *channelResp
			if len(users) > 0 && msg.FromUID != "" {
				for _, user := range users {
					if msg.FromUID == user.UID {
						fromChannel = &channelResp{
							ChannelID:     user.UID,
							ChannelType:   common.ChannelTypePerson.Uint8(),
							ChannelRemark: user.Remark,
							ChannelName:   user.Name,
						}
					}
				}
			}
			if msg.ChannelType == common.ChannelTypeGroup.Uint8() {
				for _, group := range groups {
					if group.GroupNo == msg.ChannelID {
						tempChannel = &channelResp{
							ChannelID:     group.GroupNo,
							ChannelType:   common.ChannelTypeGroup.Uint8(),
							ChannelName:   group.Name,
							ChannelRemark: group.Remark,
						}
						break
					}
				}
			}
			messagesResp = append(messagesResp, &messageResp{
				MessageIDStr: msg.MessageIDStr,
				MessageID:    msg.MessageID,
				MessageSeq:   msg.MessageSeq,
				FromUID:      msg.FromUID,
				Timestamp:    msg.Timestamp,
				Payload:      payloadMap,
				ClientMsgNo:  msg.ClientMsgNo,
				Channel:      tempChannel,
				IsDeleted:    isDeleted,
				FromChannel:  fromChannel,
			})
		}
	}
	c.Response(map[string]interface{}{
		"friends":  friendResps,
		"groups":   groupResps,
		"messages": messagesResp,
	})
}

type channelResp struct {
	ChannelID     string `json:"channel_id"`
	ChannelType   uint8  `json:"channel_type"`
	ChannelRemark string `json:"channel_remark"`
	ChannelName   string `json:"channel_name"`
}

type messageResp struct {
	Setting      uint8                  `json:"setting"`           // 设置
	MessageID    int64                  `json:"message_id"`        // 服务端的消息ID(全局唯一)
	MessageIDStr string                 `json:"message_idstr"`     // 服务端的消息ID(全局唯一)字符串形式
	MessageSeq   uint32                 `json:"message_seq"`       // 消息序列号 （用户唯一，有序递增）
	ClientMsgNo  string                 `json:"client_msg_no"`     // 客户端消息唯一编号
	FromUID      string                 `json:"from_uid"`          // 发送者UID
	Expire       uint32                 `json:"expire,omitempty"`  // expire
	Timestamp    int32                  `json:"timestamp"`         // 服务器消息时间戳(10位，到秒)
	Payload      map[string]interface{} `json:"payload"`           // 消息内容
	IsDeleted    int8                   `json:"is_deleted"`        // 是否已删除
	Channel      *channelResp           `json:"channel,omitempty"` // 消息所属channel
	FromChannel  *channelResp           `json:"from_channel"`      // 消息发送者channel
}
