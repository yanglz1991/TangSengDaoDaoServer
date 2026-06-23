-- +migrate Up

ALTER TABLE `app_config` ADD COLUMN sms_verify_on smallint not null DEFAULT 1 COMMENT '是否开启短信验证码';
