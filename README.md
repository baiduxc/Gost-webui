# GOST 面板

基于 [gost](https://github.com/go-gost/gost) 的**中转与落地管理面板**：一台服务器一条命令装好，
既能粘贴已有落地链接做中转，也能把当前服务器或远程服务器直接配置成 GOST 落地；
同时提供流量统计、流量配额、限速、Telegram 告警和 Web 管理界面。

```
客户端 ──① 使用面板生成的新链接──► 中转机（面板 + gost） ──② 原始转发──► 落地机（xray/v2ray 等）
客户端 ───────────────────────────► 本机落地（面板直接管理 GOST 代理服务）
```

## 功能

- **一键安装**：自动安装 Go、gost、面板，并注册 systemd 服务
- **粘贴即用**：支持 `vmess://` `vless://` `trojan://` `ss://` `hysteria2://` `tuic://`，
  自动解析参数、自动补 SNI/Host、UDP 协议自动开启 UDP 转发，并生成新链接与二维码
- **本机也能落地**：添加节点时选择「GOST 体系 → 本机」，HTTP、SOCKS、SS、Relay、SNI 服务直接加入面板管理的 GOST 进程
- **完整 GOST 协议层**：代理协议与传输通道独立选择，覆盖 TCP/UDP、TLS/DTLS、WS、HTTP/2、gRPC、KCP、QUIC、HTTP/3、WebTransport、SSH、ICMP 等官方通道
- **流量统计**：今日 / 本月 / 累计流量、连接数、节点流量曲线
- **流量配额**：每日 / 每月 / 总量，双向或单向计数，**超限自动暂停、新周期自动恢复**
- **限速 / 并发限制**：按节点限制上下行速率与最大连接数
- **Telegram 通知**：节点上下线、流量预警、CPU/内存/磁盘负载、面板启停
- **Web 管理**：改端口、改访问路径、重启面板、查看日志、一键开关节点
- **SSH 菜单**：登录服务器输入 `go-ui`，可改账号密码/端口/路径、重启、干净卸载

## 一键安装

**环境要求**：Linux（amd64 / arm64）、root 权限、服务器可以访问 GitHub。

```bash
curl -fsSL https://raw.githubusercontent.com/baiduxc/gost-webui/main/install.sh | sudo bash
```

安装完成后会打印面板地址、用户名和登录密码。默认监听 `8787` 端口。

**自定义端口 / 账号密码 / 访问路径**：

```bash
curl -fsSL https://raw.githubusercontent.com/baiduxc/gost-webui/main/install.sh \
  | sudo bash -s -- --port 9000 --path /panel --user admin --pass '你的密码'
```

**已下载源码包则直接运行**（会交互式询问端口、路径、用户名、密码）：

```bash
cd gost-webui && sudo bash install.sh
```

**无法访问 GitHub 的服务器**：在其它机器下载好两个二进制后上传，再指定安装

```bash
sudo bash install.sh --gost-bin /root/gost --panel-bin /root/gost-webui
```

> 安装过程会自动：放行本机防火墙端口、探测公网 IP、生成随机密码、注册 systemd 服务。
> 云服务器还需要在**控制台安全组**放行面板端口（以及转发节点使用的端口）。

## Docker 部署

镜像已内置面板与转发引擎 gost（源码编译，支持 vmess 等全协议），amd64 / arm64 多架构。

```bash
docker run -d --name gost-webui \
  --restart unless-stopped \
  -p 8787:8787 \
  -e ADMIN_USER=admin \
  -e ADMIN_PASSWORD=*** \
  -v gost-data:/var/lib/gost-webui \
  -v gost-logs:/var/log/gost-webui \
  -v gost-conf:/etc/gost-webui \
  ghcr.io/baiduxc/gost-webui:latest
```

打开 `http://服务器IP:8787` 用上面设置的账号登录。节点转发端口按需再加 `-p 45678:45678`。

或使用仓库中的 compose 文件（编辑好密码后）：

```bash
docker compose up -d
```

| 环境变量 | 说明 |
|---|---|
| `ADMIN_USER` / `ADMIN_PASSWORD` | 首次启动创建管理员；已有数据卷时以面板内设置为准 |
| `PUBLIC_HOST` | 节点连接地址对外展示用（如域名或 IP） |
| `GOST_LOG_LEVEL` | 转发引擎日志级别 `trace/debug/info/warn/error/off`，默认 info |
| `LISTEN` | 改面板监听端口（写进配置，与 `-p` 映射保持一致） |

数据（账号、节点、流量统计）在 `gost-data` 卷；升级只需 `docker pull` 后重建容器。

## 快速上手

1. 浏览器打开 `http://服务器IP:8787`，用安装时打印的账号密码登录
2. 进入 **节点** → **添加节点**
3. 选择一种方式：
   - **粘贴链接**：粘贴已有落地机的 v2rayN 分享链接
   - **GOST 体系**：选择本机或远程落地，再选择代理协议与传输通道
4. 设置监听端口（可点「随机」），保存后复制生成的客户端地址

标准 `HTTP`、`SOCKS`、`Shadowsocks` 地址可交给兼容客户端；带 `+ws`、`+grpc`、`+quic` 等组合的地址可直接用于 `gost -F`。

## 常用操作

```bash
go-ui                 # SSH 管理菜单：改密码/端口/路径、重启、日志、卸载
go-ui status          # 查看运行状态
go-ui passwd          # 修改管理员账号密码
```

```bash
systemctl status gost-webui     # 服务状态
systemctl restart gost-webui    # 重启面板
journalctl -u gost-webui -f     # 查看日志
```

## 卸载

```bash
sudo bash /opt/gost-webui/install.sh --uninstall
# 保留数据：KEEP_DATA=1 sudo bash /opt/gost-webui/install.sh --uninstall
```

也可以在 SSH 里输入 `go-ui` → 选择「8) 干净卸载」。

## 常见问题

**面板打不开？**
检查三处：① 云控制台安全组放行面板端口；② 本机防火墙（安装脚本已尝试自动放行）；
③ `ss -lntp | grep 8787` 确认监听正常。

**客户端连不上？**
到「系统设置 → 服务器地址」填写服务器的**公网 IP 或域名**（填内网 IP 无法从外网连接）。

**收不到 Telegram 通知？**
检查 Bot Token、Chat ID 是否正确，以及服务器能否访问 `api.telegram.org`；
不通时可把「API 地址」改成自建反代地址。

**忘记面板密码？**
SSH 登录后执行 `go-ui` → 选择「2) 修改管理员账号密码」重置。

## 免责声明与合规提示

- 本项目是**开源软件**，与 [gost](https://github.com/go-gost/gost) 官方无隶属或背书关系，
  仅供学习研究与**合法用途**（如自有服务器的端口转发、内网穿透、流量统计与告警）。
- 软件按「现状」提供，不附带任何明示或暗示担保；使用本软件产生的任何后果由使用者自行承担。
- **请遵守所在国家/地区的法律法规。** 在中国大陆，未经许可搭建、使用跨境信道访问境外网络，
  以及向他人提供/出售此类服务，可能违反《计算机信息网络国际联网管理暂行规定》等法规，情节严重的
  可能承担刑事责任。请勿将本项目用于上述用途；也不要用它转发、传播违法内容。
- 本项目**不收集任何遥测数据**：服务器指标只在本机展示，或仅推送给你自己配置的 Telegram 机器人。
- 公网部署请务必：修改默认密码、通过反向代理启用 HTTPS 或限制访问来源、及时更新版本。

## 相关文档

- 架构设计、配置项、开发调试、打包发布、详细排障：[DEVELOPMENT.md](DEVELOPMENT.md)
- 第三方组件与许可证：[THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md)

## License

本项目以 [MIT](LICENSE) 许可证开源。第三方组件许可证见 [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md)。
