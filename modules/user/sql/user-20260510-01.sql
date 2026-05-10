

-- +migrate Up

-- IP 黑名单：被加入黑名单的 IP 无法登录任何账号
CREATE TABLE `ip_blacklist` (
    `id` BIGINT(20) NOT NULL AUTO_INCREMENT,
    `ip` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '被封禁的 IP',
    `reason` VARCHAR(255) NOT NULL DEFAULT '' COMMENT '封禁原因（管理后台填写）',
    `operator_uid` VARCHAR(40) NOT NULL DEFAULT '' COMMENT '执行封禁的管理员 uid',
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_ip_blacklist_ip` (`ip`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='登录 IP 黑名单';

-- 设备黑名单：被加入黑名单的 device_id 无法登录任何账号
CREATE TABLE `device_blacklist` (
    `id` BIGINT(20) NOT NULL AUTO_INCREMENT,
    `device_id` VARCHAR(128) NOT NULL DEFAULT '' COMMENT '被封禁的设备唯一 ID',
    `reason` VARCHAR(255) NOT NULL DEFAULT '' COMMENT '封禁原因（管理后台填写）',
    `operator_uid` VARCHAR(40) NOT NULL DEFAULT '' COMMENT '执行封禁的管理员 uid',
    `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_device_blacklist_device_id` (`device_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='登录设备黑名单';
