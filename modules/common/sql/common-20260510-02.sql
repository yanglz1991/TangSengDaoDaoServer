-- +migrate Up

ALTER TABLE `app_config` ADD COLUMN disable_group_message_on smallint not null DEFAULT 0 COMMENT '是否开启群聊禁言（含群发消息/建群/加群成员/加好友）';
ALTER TABLE `app_config` ADD COLUMN disable_private_message_on smallint not null DEFAULT 0 COMMENT '是否开启私聊禁言（禁止私聊发消息）';
ALTER TABLE `app_config` ADD COLUMN mute_text_of_group varchar(255) not null DEFAULT '' COMMENT '群聊禁言客户端展示文案';
ALTER TABLE `app_config` ADD COLUMN mute_text_of_private varchar(255) not null DEFAULT '' COMMENT '私聊禁言客户端展示文案';
