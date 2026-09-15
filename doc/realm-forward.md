# Realm 转发工具（第一版）

本分支在原有 GOST 转发之外增加 Realm。已有规则和未指定 `engine` 的接口请求仍使用 GOST。

## 使用范围

- 仅管理员可以创建或切换 Realm 规则。
- 仅支持“端口转发”隧道，目标地址只能有一个。
- 同时监听 TCP 和 UDP。
- 暂不提供单条规则流量统计、用户限速、负载策略和指定出口网卡。
- 节点必须是使用 systemd 的 Linux 主机；节点容器模式暂不支持 Realm。

面板的转发创建/编辑窗口中会出现“转发工具”。选择 Realm 后，以上限制会直接显示并在前后端同时校验。

## 安装与更新

使用本分支的 `install.sh` 安装或更新节点。脚本会下载 Realm v2.9.6 的官方 musl 构建，校验固定 SHA-256 后安装到：

```text
/usr/local/lib/flux-panel/realm
```

每条规则使用独立的 systemd 单元和配置：

```text
/etc/systemd/system/flux-realm-<规则ID>.service
/var/lib/flux-panel/realm/<规则ID>.json
```

创建、编辑、暂停、恢复和删除规则都通过节点现有 WebSocket 控制通道执行。节点端只接受固定动作和经过校验的规则 ID、端口及目标地址，不执行面板传入的 shell 命令。更新节点时会重启现有 Realm 单元以加载新二进制。

## 现有数据库升级

`panel_install.sh` 的更新流程会自动增加 `forward.engine` 字段，并把历史记录设为 `gost`。如果是手工部署，只需执行一次：

```sql
ALTER TABLE `forward`
  ADD COLUMN `engine` varchar(20) NOT NULL DEFAULT 'gost' AFTER `strategy`;
```

对应脚本位于 `doc/migrations/001_forward_engine.sql`。全新部署使用更新后的 `gost.sql`，无需另行迁移。

## 故障检查

规则创建失败时，面板会显示节点返回的错误。可在对应节点查看：

```bash
systemctl status flux-realm-<规则ID>.service
journalctl -u flux-realm-<规则ID>.service
```

更新规则时，如果新配置启动失败，节点会尝试恢复此前的配置；在面板中切换隧道或转发工具失败时，主控端也会尝试恢复旧运行时。
