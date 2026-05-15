
-- +migrate Up

-- 用户「加好友/创建群聊」权限开关
--   - 1: 允许 (主动发起好友申请、创建群聊)
--   - 0: 禁止 (默认)
-- 影响范围：
--   POST /v1/friend/apply  (主动发起好友申请)
--   POST /v1/group/create  (创建群聊)
-- 不影响：被动接受好友请求、被动入群、邀请进群（按业务保留）。
-- 新注册用户（手机号/用户名/第三方）默认 0，由管理后台批量生成或单独切换为 1。
-- superAdmin / admin 角色服务端跳过该字段校验。
ALTER TABLE `user` ADD COLUMN `can_invite_or_create_group` SMALLINT NOT NULL DEFAULT 0
    COMMENT '是否允许主动加好友/创建群聊 0.否 1.是';

-- 老用户兼容：迁移执行时已存在的用户一次性设为 1（保持原有可用），
-- 仅在此之后新注册的用户使用 DEFAULT 0（按需求「手机号注册默认无加人/建群权限」）。
UPDATE `user` SET `can_invite_or_create_group` = 1;
