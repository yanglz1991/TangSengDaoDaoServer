package common

import (
	dbs "github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
)

// 加密通道配置(单条记录)
type secureChannelModel struct {
	Name     string // 按钮名称
	URL      string // H5 跳转地址
	Password string // 访问密码
	Enabled  int    // 是否启用 0:停用 1:启用
	dbs.BaseModel
}

// 查询加密通道配置(取最新一条)
func (d *db) querySecureChannel() (*secureChannelModel, error) {
	var m *secureChannelModel
	_, err := d.session.Select("*").From("secure_channel").OrderDir("id", false).Limit(1).Load(&m)
	return m, err
}

// 新增配置
func (d *db) insertSecureChannel(m *secureChannelModel) (int64, error) {
	result, err := d.session.InsertInto("secure_channel").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// 按 id 更新配置
func (d *db) updateSecureChannelByID(id int64, name, url, password string, enabled int) error {
	_, err := d.session.Update("secure_channel").
		Set("name", name).
		Set("url", url).
		Set("password", password).
		Set("enabled", enabled).
		Where("id=?", id).
		Exec()
	return err
}
