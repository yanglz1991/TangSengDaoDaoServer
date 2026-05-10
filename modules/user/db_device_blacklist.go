package user

import (
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/gocraft/dbr/v2"
)

// deviceBlacklistDB 登录设备黑名单 DB
type deviceBlacklistDB struct {
	session *dbr.Session
	ctx     *config.Context
}

func newDeviceBlacklistDB(ctx *config.Context) *deviceBlacklistDB {
	return &deviceBlacklistDB{session: ctx.DB(), ctx: ctx}
}

// upsert 插入或忽略
func (d *deviceBlacklistDB) upsert(m *deviceBlacklistModel) error {
	_, err := d.session.InsertBySql(
		"insert into device_blacklist(device_id, reason, operator_uid) values(?, ?, ?) ON DUPLICATE KEY UPDATE reason=VALUES(reason), operator_uid=VALUES(operator_uid)",
		m.DeviceID, m.Reason, m.OperatorUID,
	).Exec()
	return err
}

// delete 解禁
func (d *deviceBlacklistDB) delete(deviceID string) error {
	_, err := d.session.DeleteFrom("device_blacklist").Where("device_id=?", deviceID).Exec()
	return err
}

// exist 是否在黑名单
func (d *deviceBlacklistDB) exist(deviceID string) (bool, error) {
	if deviceID == "" {
		return false, nil
	}
	var count int
	_, err := d.session.Select("count(*)").From("device_blacklist").Where("device_id=?", deviceID).Load(&count)
	return count > 0, err
}

// queryByDeviceIDs 批量查询某些设备的黑名单状态
func (d *deviceBlacklistDB) queryByDeviceIDs(deviceIDs []string) ([]*deviceBlacklistModel, error) {
	if len(deviceIDs) == 0 {
		return nil, nil
	}
	var list []*deviceBlacklistModel
	_, err := d.session.Select("*").From("device_blacklist").Where("device_id in ?", deviceIDs).Load(&list)
	return list, err
}

type deviceBlacklistModel struct {
	DeviceID    string
	Reason      string
	OperatorUID string
	db.BaseModel
}
