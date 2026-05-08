-- +migrate Up

CREATE TABLE `secure_channel` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `name` varchar(80) NOT NULL DEFAULT '' COMMENT '加密通道按钮名称',
  `url` varchar(500) NOT NULL DEFAULT '' COMMENT 'H5 跳转地址',
  `password` varchar(80) NOT NULL DEFAULT '' COMMENT '访问密码',
  `enabled` smallint NOT NULL DEFAULT 0 COMMENT '是否启用 0:停用 1:启用',
  `created_at` timeStamp not null DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` timeStamp not null DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='加密通道配置';

INSERT INTO `secure_channel` (`name`, `url`, `password`, `enabled`) VALUES ('', '', '', 0);
