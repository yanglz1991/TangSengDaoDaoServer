
-- +migrate Up

-- ============================================================================
-- 层级管理
--   层级（level_node）以 node_no 为唯一编号，parent_no 为父层级编号（顶层为 ""）
--   每个层级可配置：负责人 owner_uid、邀请码 invite_code（全局唯一，用于注册）、
--   多个默认好友 level_default_friend（注册后自动互为好友）
--
-- 路由：/v1/manager/level/...
-- 注册流程：
--   user.register 流程会通过 BussDataSource.GetInviteCode 命中 level 模块；
--   user 模块持续触发 EventUserRegister 事件并把 invite_code 透传给 level，
--   level 监听器再把 user.level_node_no 设置为对应层级，并加默认好友。
-- ============================================================================

CREATE TABLE IF NOT EXISTS `level_node` (
    `id` BIGINT(20) NOT NULL AUTO_INCREMENT,
    `node_no` VARCHAR(40) NOT NULL DEFAULT '' COMMENT '层级唯一编号',
    `parent_no` VARCHAR(40) NOT NULL DEFAULT '' COMMENT '父层级编号 空表示顶层',
    `name` VARCHAR(80) NOT NULL DEFAULT '' COMMENT '层级名称',
    `owner_uid` VARCHAR(40) NOT NULL DEFAULT '' COMMENT '层级负责人uid',
    `invite_code` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '邀请码 全局唯一',
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_node_no` (`node_no`),
    UNIQUE KEY `idx_invite_code` (`invite_code`),
    KEY `idx_parent_no` (`parent_no`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='层级';

CREATE TABLE IF NOT EXISTS `level_default_friend` (
    `id` BIGINT(20) NOT NULL AUTO_INCREMENT,
    `node_no` VARCHAR(40) NOT NULL DEFAULT '' COMMENT '层级编号',
    `friend_uid` VARCHAR(40) NOT NULL DEFAULT '' COMMENT '默认好友uid',
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_node_friend` (`node_no`,`friend_uid`),
    KEY `idx_friend_uid` (`friend_uid`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='层级默认好友';

-- 用户所属的层级节点编号；空表示未挂在任何层级
-- ponytail: MySQL 不支持 ADD COLUMN IF NOT EXISTS，用存储过程做幂等；升级到 MariaDB 后可简化
DROP PROCEDURE IF EXISTS _add_level_node_no;
-- +migrate StatementBegin
CREATE PROCEDURE _add_level_node_no()
BEGIN
    IF EXISTS (SELECT * FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'user') THEN
        IF NOT EXISTS (SELECT * FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'user' AND COLUMN_NAME = 'level_node_no') THEN
            ALTER TABLE `user` ADD COLUMN `level_node_no` VARCHAR(40) NOT NULL DEFAULT '' COMMENT '所属层级节点';
        END IF;
        IF NOT EXISTS (SELECT * FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'user' AND INDEX_NAME = 'idx_user_level_node_no') THEN
            ALTER TABLE `user` ADD INDEX `idx_user_level_node_no` (`level_node_no`);
        END IF;
    END IF;
END;
-- +migrate StatementEnd
CALL _add_level_node_no();
DROP PROCEDURE IF EXISTS _add_level_node_no;
