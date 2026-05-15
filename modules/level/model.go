package level

import "github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"

// nodeModel 层级节点
type nodeModel struct {
	NodeNo     string
	ParentNo   string
	Name       string
	OwnerUID   string
	InviteCode string
	db.BaseModel
}

// defaultFriendModel 层级默认好友
type defaultFriendModel struct {
	NodeNo    string
	FriendUID string
	db.BaseModel
}

// nodeDetail 用于树形/详情返回
type nodeDetail struct {
	NodeNo     string `json:"node_no"`
	ParentNo   string `json:"parent_no"`
	Name       string `json:"name"`
	OwnerUID   string `json:"owner_uid"`
	OwnerName  string `json:"owner_name"`
	InviteCode string `json:"invite_code"`
	UserCount  int64  `json:"user_count"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// nodeUserResp 层级下用户
type nodeUserResp struct {
	UID                    string `json:"uid"`
	Name                   string `json:"name"`
	Phone                  string `json:"phone"`
	Username               string `json:"username"`
	ShortNo                string `json:"short_no"`
	Status                 int    `json:"status"`
	CanInviteOrCreateGroup int    `json:"can_invite_or_create_group"`
	RegisterTime           string `json:"register_time"`
	LevelNodeNo            string `json:"level_node_no"`
}
