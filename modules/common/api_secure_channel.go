package common

import (
	"errors"
	"strings"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"go.uber.org/zap"
)

// 加密通道配置 - 普通用户读取(不含 url 与 password)
func (cn *Common) secureChannelGet(c *wkhttp.Context) {
	m, err := cn.db.querySecureChannel()
	if err != nil {
		cn.Error("查询加密通道配置失败", zap.Error(err))
		c.ResponseError(errors.New("查询加密通道配置失败"))
		return
	}
	enabled := false
	name := ""
	if m != nil && m.Enabled == 1 && strings.TrimSpace(m.URL) != "" {
		enabled = true
		name = m.Name
	}
	c.Response(map[string]interface{}{
		"enabled": enabled,
		"name":    name,
	})
}

// 加密通道配置 - 验证密码,通过则下发 url
func (cn *Common) secureChannelVerify(c *wkhttp.Context) {
	type ReqVO struct {
		Password string `json:"password"`
	}
	var req ReqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	m, err := cn.db.querySecureChannel()
	if err != nil {
		cn.Error("查询加密通道配置失败", zap.Error(err))
		c.ResponseError(errors.New("查询加密通道配置失败"))
		return
	}
	if m == nil || m.Enabled != 1 || strings.TrimSpace(m.URL) == "" {
		c.ResponseError(errors.New("加密通道未开启"))
		return
	}
	if req.Password != m.Password {
		c.ResponseError(errors.New("密码错误"))
		return
	}
	c.Response(map[string]interface{}{
		"url": m.URL,
	})
}
