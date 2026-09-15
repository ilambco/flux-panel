# Flux Panel 转发面板

这是基于原版“哆啦A梦转发面板”维护的个人分支。本项目基于 [go-gost/gost](https://github.com/go-gost/gost) 和 [go-gost/x](https://github.com/go-gost/x)，用于集中管理转发节点和规则。

仓库使用两个发布通道：`develop` 用于边开发边部署测试，`main` 用于验证通过后的稳定版本。每次推送后，GitHub Actions 会自动检查代码、构建面板镜像和节点程序。
---
## 特性

- 支持按 **隧道账号级别** 管理流量转发数量，可用于用户/隧道配额控制
- 支持 **TCP** 和 **UDP** 协议的转发
- 支持两种转发模式：**端口转发** 与 **隧道转发**
- 可针对 **指定用户的指定隧道进行限速** 设置
- 支持配置 **单向或双向流量计费方式**，灵活适配不同计费模型
- 提供灵活的转发策略配置，适用于多种网络场景
- 管理员可为单节点端口转发选择 **Realm** 运行时（详见 [Realm 转发工具](doc/realm-forward.md)）


## 部署流程
---
### Docker Compose 部署

#### 稳定版（main）

面板端：
```bash
curl -fsSL https://github.com/ilambco/flux-panel/releases/download/channel-main/panel_install.sh -o panel_install.sh && chmod +x panel_install.sh && FLUX_CHANNEL=main ./panel_install.sh
```

节点端：
```bash
curl -fsSL https://github.com/ilambco/flux-panel/releases/download/channel-main/install.sh -o install.sh && chmod +x install.sh && FLUX_CHANNEL=main ./install.sh
```

#### 测试版（develop）

首次安装面板：
```bash
curl -fsSL https://github.com/ilambco/flux-panel/releases/download/channel-develop/panel_install.sh -o panel_install.sh && chmod +x panel_install.sh && FLUX_CHANNEL=develop ./panel_install.sh
```

更新已部署的面板（可在任意目录执行）：
```bash
curl -fsSL https://github.com/ilambco/flux-panel/releases/download/channel-develop/panel_install.sh -o /tmp/flux-panel-update.sh && FLUX_CHANNEL=develop bash /tmp/flux-panel-update.sh update
```

更新节点程序：
```bash
curl -fsSL https://github.com/ilambco/flux-panel/releases/download/channel-develop/install.sh -o /tmp/flux-node-update.sh && FLUX_CHANNEL=develop bash /tmp/flux-node-update.sh update
```

面板更新前会在部署目录的 `.flux-backups` 中保存配置和数据库。执行更新前，请先确认仓库 Actions 页面中 `develop` 的最新构建已经成功。

#### 默认管理员账号

- **账号**: admin_user
- **密码**: admin_user

> ⚠️ 首次登录后请立即修改默认密码！


## 免责声明

本项目仅供个人学习与研究使用，基于开源项目进行二次开发。  

使用本项目所带来的任何风险均由使用者自行承担，包括但不限于：  

- 配置不当或使用错误导致的服务异常或不可用；  
- 使用本项目引发的网络攻击、封禁、滥用等行为；  
- 服务器因使用本项目被入侵、渗透、滥用导致的数据泄露、资源消耗或损失；  
- 因违反当地法律法规所产生的任何法律责任。  

本项目为开源的流量转发工具，仅限合法、合规用途。  
使用者必须确保其使用行为符合所在国家或地区的法律法规。  

**作者不对因使用本项目导致的任何法律责任、经济损失或其他后果承担责任。**  
**禁止将本项目用于任何违法或未经授权的行为，包括但不限于网络攻击、数据窃取、非法访问等。**  

如不同意上述条款，请立即停止使用本项目。  

作者对因使用本项目所造成的任何直接或间接损失概不负责，亦不提供任何形式的担保、承诺或技术支持。  


请务必在合法、合规、安全的前提下使用本项目。  

---
## ⭐ 喝杯咖啡！（USDT）

| 网络       | 地址                                                                 |
|------------|----------------------------------------------------------------------|
| BNB(BEP20) | `0x755492c03728851bbf855daa28a1e089f9aca4d1`                          |
| TRC20      | `TYh2L3xxXpuJhAcBWnt3yiiADiCSJLgUm7`                                  |
| Aptos      | `0xf2f9fb14749457748506a8281628d556e8540d1eb586d202cd8b02b99d369ef8`  |

[![Star History Chart](https://api.star-history.com/svg?repos=ilambco/flux-panel&type=Date)](https://www.star-history.com/#ilambco/flux-panel&Date)

