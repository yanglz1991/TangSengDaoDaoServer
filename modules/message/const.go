package message

const (
	// 消息已删除
	CMDMessageDeleted = "messageDeleted"
	// CMDMessageErase 消息擦除
	CMDMessageErase       = "messageEerase"
	sensitiveWordsVersion = 2
)
const CacheReadedCountPrefix = "readedCount:" // 消息已读数量

type ReminderType int

const (
	ReminderTypeMentionMe      = 1 // 有人@我
	ReminderTypeApplyJoinGroup = 2 // 申请加群
)

// 已停用：聊天敏感词提示（"涉及私下交易、转账等资金问题…"）已下线，
// 故关键词列表清空，syncSensitiveWords 接口将返回空列表与空 tips。
var sensitive_words = []string{}
