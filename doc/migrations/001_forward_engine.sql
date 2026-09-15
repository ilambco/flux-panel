-- Run once when upgrading an existing database.
ALTER TABLE `forward`
  ADD COLUMN `engine` varchar(20) NOT NULL DEFAULT 'gost' AFTER `strategy`;
