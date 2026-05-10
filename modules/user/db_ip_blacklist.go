package user

import (
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/gocraft/dbr/v2"
)

// ipBlacklistDB 登录 IP 黑名单 DB
type ipBlacklistDB struct {
	session *dbr.Session
	ctx     *config.Context
}

func newIPBlacklistDB(ctx *config.Context) *ipBlacklistDB {
	return &ipBlacklistDB{session: ctx.DB(), ctx: ctx}
}

// upsert 插入或忽略（同一个 IP 重复封禁视为同一条）
func (d *ipBlacklistDB) upsert(m *ipBlacklistModel) error {
	_, err := d.session.InsertBySql(
		"insert into ip_blacklist(ip, reason, operator_uid) values(?, ?, ?) ON DUPLICATE KEY UPDATE reason=VALUES(reason), operator_uid=VALUES(operator_uid)",
		m.IP, m.Reason, m.OperatorUID,
	).Exec()
	return err
}

// delete 解禁
func (d *ipBlacklistDB) delete(ip string) error {
	_, err := d.session.DeleteFrom("ip_blacklist").Where("ip=?", ip).Exec()
	return err
}

// exist 是否在黑名单
func (d *ipBlacklistDB) exist(ip string) (bool, error) {
	if ip == "" {
		return false, nil
	}
	var count int
	_, err := d.session.Select("count(*)").From("ip_blacklist").Where("ip=?", ip).Load(&count)
	return count > 0, err
}

// queryByIPs 批量查询某些 IP 的黑名单状态
func (d *ipBlacklistDB) queryByIPs(ips []string) ([]*ipBlacklistModel, error) {
	if len(ips) == 0 {
		return nil, nil
	}
	var list []*ipBlacklistModel
	_, err := d.session.Select("*").From("ip_blacklist").Where("ip in ?", ips).Load(&list)
	return list, err
}

type ipBlacklistModel struct {
	IP          string
	Reason      string
	OperatorUID string
	db.BaseModel
}
