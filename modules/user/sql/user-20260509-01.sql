

-- +migrate Up

-- 为 login_log 增加索引，支撑：
--   1. queryLastLoginIPWithUids 中 `group by uid having uid in (...)` 的批量取最后登录 IP
--   2. queryLastLoginIP 中按 uid 倒序取最近一条
--   3. 管理端用户列表按 IP 关键字搜索 (select uid from login_log where login_ip like ?)
-- (login_ip, uid) 组合在搜索子查询时可作为覆盖索引（projection 只取 uid），减少回表
CREATE INDEX `login_log_uid` on `login_log` (`uid`);
CREATE INDEX `login_log_login_ip_uid` on `login_log` (`login_ip`, `uid`);

-- 为 device 增加按设备名/设备型号搜索的覆盖索引（projection 只取 uid，减少回表）
-- device(uid) 已在 user-20191106-01.sql 中存在，无需重复
CREATE INDEX `device_device_name_uid` on `device` (`device_name`, `uid`);
CREATE INDEX `device_device_model_uid` on `device` (`device_model`, `uid`);
