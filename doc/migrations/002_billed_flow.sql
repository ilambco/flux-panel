-- 为三种流量计费模式增加独立累计字段。
-- 首次迁移时用旧版已记录的入站+出站总和初始化，避免升级后配额用量归零。

ALTER TABLE `forward`
  ADD COLUMN `used_flow` BIGINT(20) NOT NULL DEFAULT 0 COMMENT '按链路计费模式累计的流量' AFTER `out_flow`;
UPDATE `forward` SET `used_flow` = `in_flow` + `out_flow`;

ALTER TABLE `user`
  ADD COLUMN `used_flow` BIGINT(20) NOT NULL DEFAULT 0 COMMENT '按链路计费模式累计的流量' AFTER `out_flow`;
UPDATE `user` SET `used_flow` = `in_flow` + `out_flow`;

ALTER TABLE `user_tunnel`
  ADD COLUMN `used_flow` BIGINT(20) NOT NULL DEFAULT 0 COMMENT '按链路计费模式累计的流量' AFTER `out_flow`;
UPDATE `user_tunnel` SET `used_flow` = `in_flow` + `out_flow`;
