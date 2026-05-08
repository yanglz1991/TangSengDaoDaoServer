package common

import (
	"errors"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/wkhttp"
	"go.uber.org/zap"
)

// 获取加密通道配置(超管)
func (m *Manager) getSecureChannel(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	model, err := m.db.querySecureChannel()
	if err != nil {
		m.Error("查询加密通道配置失败", zap.Error(err))
		c.ResponseError(errors.New("查询加密通道配置失败"))
		return
	}
	if model == nil {
		c.Response(map[string]interface{}{
			"name":     "",
			"url":      "",
			"password": "",
			"enabled":  0,
		})
		return
	}
	c.Response(map[string]interface{}{
		"name":     model.Name,
		"url":      model.URL,
		"password": model.Password,
		"enabled":  model.Enabled,
	})
}

// 更新加密通道配置(超管)
func (m *Manager) updateSecureChannel(c *wkhttp.Context) {
	err := c.CheckLoginRoleIsSuperAdmin()
	if err != nil {
		c.ResponseError(err)
		return
	}
	type ReqVO struct {
		Name     string `json:"name"`
		URL      string `json:"url"`
		Password string `json:"password"`
		Enabled  int    `json:"enabled"`
	}
	var req ReqVO
	if err := c.BindJSON(&req); err != nil {
		c.ResponseError(errors.New("请求数据格式有误！"))
		return
	}
	if req.Enabled != 0 && req.Enabled != 1 {
		req.Enabled = 0
	}

	existing, err := m.db.querySecureChannel()
	if err != nil {
		m.Error("查询加密通道配置失败", zap.Error(err))
		c.ResponseError(errors.New("查询加密通道配置失败"))
		return
	}
	if existing == nil {
		_, err = m.db.insertSecureChannel(&secureChannelModel{
			Name:     req.Name,
			URL:      req.URL,
			Password: req.Password,
			Enabled:  req.Enabled,
		})
		if err != nil {
			m.Error("新增加密通道配置失败", zap.Error(err))
			c.ResponseError(errors.New("新增加密通道配置失败"))
			return
		}
	} else {
		err = m.db.updateSecureChannelByID(existing.Id, req.Name, req.URL, req.Password, req.Enabled)
		if err != nil {
			m.Error("更新加密通道配置失败", zap.Error(err))
			c.ResponseError(errors.New("更新加密通道配置失败"))
			return
		}
	}
	c.ResponseOK()
}
