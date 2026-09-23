# GOST 面板 · 开发与运维文档

> 本文件面向开发者/运维（架构、配置项、发布流程、排障）。
> **面向使用者的安装说明请看 [README.md](README.md)。**

基于 [gost](https://github.com/go-gost/gost) 的中转与落地 WebUI 面板：既可粘贴 v2rayN 链接做透明中转，也可由面板在本机或远程落地机创建 GOST 原生代理服务。
自带流量统计、流量配额（超限自动暂停、到期自动恢复）、限速与连接数限制。

```
        ┌──────────────┐        ┌───────────────────────────┐        ┌──────────────┐
        │   客户端      │  ①    │        中转机 (本面板)      │   ②    │   落地机      │
        │  v2rayN 等   │ ─────► │  gost: :中转端口 → 落地地址  │ ─────► │ xray/v2ray   │
        └──────────────┘        │  面板: 统计/配额/限速        │        │ vmess/vless  │
                                └───────────────────────────┘        └──────────────┘
   ① 客户端使用面板生成的「中转版链接」（地址=中转机，其余参数与落地机一致）
   ② 中转机对落地机做原始 TCP/UDP 转发，TLS / WS / gRPC / Reality 等全部原样透传
```

## 特性

- **一键安装**：脚本自动装 Go（如需编译）、编译/下载 gost、部署面板并注册 systemd 服务
- **粘贴即用**：粘贴落地机的 `vmess://` `vless://` `trojan://` `ss://` `hysteria2://` `tuic://` 链接，
  自动解析协议、地址、端口、UUID/密码、TLS/SNI/WS 等参数，并生成客户端新链接（含二维码）
- **TCP + UDP 转发**：hysteria2 / tuic / KCP / QUIC 等 UDP 协议自动开启 UDP 转发
- **本机落地**：同一台服务器上的 GOST 进程可直接运行代理服务，不再需要第二台落地机或额外 systemd 服务
- **GOST 原生协议**：覆盖官方公网代理处理器（HTTP/2、SOCKS4/4A/5、SS/SSU、SNI、Relay）及 TCP/UDP、TLS、WS、gRPC、KCP、QUIC、HTTP/3、SSH、ICMP 等网络通道
- **流量统计**：实时连接数、今日/本月/累计流量、节点流量曲线（24 小时 / 7 天 / 30 天）
- **流量配额**：每日 / 每月 / 总量，双向/单向计数，**达到额度自动暂停，周期结束自动恢复**
- **限速与并发限制**：按节点设置上下行限速（Mbps）、最大并发连接数
- **落地链接智能改写**：自动为 TLS 补全 SNI、为 WS 补全 Host，避免改地址后握手失败
- **无需重启**：通过 gost REST API 热增删服务，新增节点不影响其它节点已有连接
- **Telegram 通知**：节点上下线、流量超限、流量预警（80%/95% 等阈值）、服务器 CPU/内存/磁盘负载提醒、面板启停通知；
  纯 HTTP 主动推送（不建长连接），支持自定义 API 反代地址与冷却限频
- **面板自助管理**：Web 界面直接修改监听端口、访问路径（如 `/panel/`）、一键重启面板
- **轻量安全**：单二进制（前端内嵌）+ bbolt 本地存储；gost API 仅监听 127.0.0.1 并带 Basic Auth；面板登录态持久化 + 登录限流

## 一键安装

在**中转机**（需要对外提供中转端口的机器）上以 root 执行：

```bash
# 方式一：已经把仓库克隆到本机（可交互输入端口/账号密码）
cd gost-webui && sudo bash install.sh

# 非交互指定配置
sudo bash install.sh --port 9000 --path /panel --user admin --pass '你的密码'
# 或环境变量：PANEL_PORT / BASE_PATH / PANEL_USER / PANEL_PASS

# 方式二：远程一键（需先发布到 GitHub，见下文「发布与一键安装」）
curl -fsSL https://raw.githubusercontent.com/baiduxc/gost-webui/main/install.sh | sudo bash
```

安装脚本会：

1. 安装基础依赖与 Go（如系统缺失）
2. 获取 gost：优先使用 GitHub Release 预编译包（经加速镜像），**并自动验证其支持面板所需的 REST API**；不可用时改为源码编译
3. 编译并安装面板到 `/opt/gost-webui/bin/gost-webui`
4. 生成配置 `/etc/gost-webui/panel.yml`（含随机面板密码与 gost API 密码）
5. 注册并启动 `gost-webui` systemd 服务（面板会守护 gost 子进程）

安装完成会打印（并安装管理菜单命令 `go-ui`）：

```
面板地址 : http://<服务器IP>:8787
用户名   : admin
登录密码 : ********
```

可选环境变量：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `PANEL_PORT` | `8787` | 面板端口（也可用 `-p/--port` 参数） |
| `PANEL_USER` / `PANEL_PASS` | admin / 随机 | 管理员账号密码（也可用 `-u/--user`、`-P/--pass`） |
| `BASE_PATH` | 空 | 面板访问路径前缀，如 `/panel`（也可登录后在系统设置中修改） |
| `PANEL_DIR` | `/opt/gost-webui` | 安装目录 |
| `GOST_BIN_URL` / `PANEL_BIN_URL` | 空 | 指定 gost / 面板二进制（本地路径或 URL），用于无法访问 GitHub 的服务器 |
| `GOPROXY` | `https://goproxy.cn,direct` | Go 模块代理 |
| `GOST_REF` | `master` | gost 源码分支/标签/提交 |
| `NO_SYSTEMD` | `0` | 设为 1 则不注册 systemd 服务 |

卸载：

```bash
sudo bash /opt/gost-webui/install.sh --uninstall
# 保留数据：KEEP_DATA=1 sudo bash install.sh --uninstall
```

## 发布与一键安装（重要）

> 本仓库地址：`https://github.com/baiduxc/gost-webui`，所有下载直连 GitHub。
> 服务器无法访问 GitHub 时，用 `--gost-bin` / `--panel-bin` 指定已下载的二进制（见下文）。

面板采用「源码仓库即安装源」的方式发布，**新服务器一条命令即可装好 gost + 面板**。

### 第 1 步：发布前改一处配置

编辑 `install.sh` 顶部的仓库地址（改成你自己的）：

```bash
DEFAULT_PANEL_REPO="https://github.com/baiduxc/gost-webui"
```

### 第 2 步：提交到 GitHub

```bash
git init && git add -A && git commit -m "release v1.1.0"
git remote add origin https://github.com/baiduxc/gost-webui.git
git push -u origin main
```

### 第 3 步（可选但推荐）：发布预编译包

预编译包让安装**不需要在服务器上编译**，速度从几分钟降到几秒：

```bash
bash release.sh            # 生成 dist/gost-webui_1.1.0_linux_amd64.tar.gz 等
gh release create v1.1.0 dist/gost-webui_1.1.0_linux_*.tar.gz --title "v1.1.0"
```

### 第 4 步：新服务器一键安装

```bash
# 方式 A：源码构建（脚本自动安装 Go、编译 gost 与面板，约 3~10 分钟）
curl -fsSL https://raw.githubusercontent.com/baiduxc/gost-webui/main/install.sh | sudo bash

# 方式 B：使用 Release 预编译包（最快，几秒钟，{arch} 按机器架构自动替换）
curl -fsSL https://raw.githubusercontent.com/baiduxc/gost-webui/main/install.sh \
  | sudo PANEL_RELEASE_URL="https://github.com/baiduxc/gost-webui/releases/download/v1.1.0/gost-webui_1.1.0_linux_{arch}.tar.gz" bash

# 自定义端口 / 账号密码 / 访问路径：
#   curl -fsSL .../install.sh | sudo bash -s -- --port 9000 --path /panel --user admin --pass 你的密码

# 无法访问 GitHub 的服务器（离线安装）：
#   在有网络的机器下载 gost 与面板二进制 → 上传到服务器 → 指定安装
#   sudo bash install.sh --gost-bin /root/gost --panel-bin /root/gost-webui
```

脚本在一台全新服务器上会自动完成：

1. 安装依赖（curl/git/tar）与 Go（如缺失）
2. 获取 gost：优先 Release 预编译包并**校验其支持所需 API**，否则源码编译
3. 安装面板：优先使用 `PANEL_RELEASE_URL` 预编译包，否则源码编译
4. 生成 `/etc/gost-webui/panel.yml`（随机面板密码与 gost API 密码），探测公网 IP 写入配置
5. 注册并启动 `gost-webui` systemd 服务（面板守护 gost 子进程）
6. 自动放行本机防火墙端口，并打印访问地址与安全组提示

## 使用流程

1. **落地机**保持原有代理服务（xray/v2ray/hysteria2…），在客户端先确认它的原始链接可用（例如在 v2rayN 里测速通过）
2. 打开面板 → **中转节点** → **+ 添加中转**
3. 把落地机链接粘贴进输入框 → 自动解析出协议/地址/端口
4. 设置**中转端口**（可点「随机」自动选空闲端口）、备注名（会作为客户端显示的节点名）
5. 按需开启**流量配额 / 限速 / 连接数限制**
6. 保存后弹出**中转版链接 + 二维码**，复制 / 扫码导入客户端即可

> 客户端连接的是 `中转机地址:中转端口`，而 TLS/SNI、WS 路径、UUID 等仍是落地机的参数，
> 因此中转机全程只做字节转发，不感知也不破解任何加密流量。

### GOST 原生落地

添加节点时切换到「GOST 体系」：

1. 选择**本机落地**时，面板把代理 handler 与 listener 直接写入当前 GOST 配置并热加载；监听端口就是客户端连接端口。
2. 选择**远程落地**时，面板生成远端 YAML、安装命令和 systemd 服务；当前机器根据传输通道使用 TCP 或 UDP 原样中转。
3. 代理协议与传输通道分层保存。旧版 GOST 节点缺少这两个字段时自动按 `ss + tcp` 兼容。
4. 标准 `ss://` 可继续用于 Clash；其它 GOST 组合会进入通用订阅，客户端地址格式为 `处理协议+传输通道://认证@主机:端口`。

SNI 透明代理本身不提供账号认证；公网使用时应限制安全组或防火墙来源。ICMP/ICMPv6/Fake TCP 使用原始报文，只允许本机落地并需要 root 或 `CAP_NET_RAW`。GOST 3.3 的 DTLS 拨号端要求客户端证书，使用时还需在客户端节点参数中配置 `certFile` / `keyFile`。

### 链接改写规则（自动完成，无需手工）

| 情况 | 处理 |
| --- | --- |
| TLS/Reality 且链接未带 SNI | 自动填入落地机原地址作为 SNI |
| WS/HTTPUpgrade 且未带 Host | 自动填入落地机原地址作为 Host |
| WS 的 `path`、gRPC 的 `serviceName`、Reality 的 `pbk/sid` 等 | 原样保留 |
| `ss://` 旧格式（整段 base64） | 自动转成 SIP002 格式 |
| hysteria2 / tuic / KCP / QUIC | 自动勾选「同时转发 UDP」 |

## 流量统计与限制说明

- **统计数据来源**：gost 内部 observer 对每个服务统计入/出字节，面板按采样间隔（默认 15 秒）计算增量并写入本地小时级明细
- **配额（流量限制）**：由 gost 的 `quota` 能力实现，计数会持久化到 `/var/lib/gost-webui/quota.json`（每 5 秒刷盘），**面板重启、gost 重启都不丢**
  - 周期：每日（每天 0 点重置）/ 每月（每月 1 号 0 点重置）/ 总量（不重置）
  - 方向：双向合计 / 仅上行 / 仅下行
  - 达到额度后：该节点的中转服务自动停止接受新连接；进入新周期后自动恢复
  - **计数兜底恢复**：若 gost 进程被强杀导致计数未落盘，面板会依据自身流量明细在秒级内把配额计数恢复回来；
    跨周期（跨天/跨月）时面板会自动切换配额窗口，让计数清零并恢复服务
- **限速**：基于 gost traffic limiter，按节点设置上行/下行速率（字节/秒，界面按 Mbps 换算）
- **连接数限制**：基于 gost climiter
- **重置流量**：节点详情 → 重置流量（清零配额计数与面板累计）
- **登录态**：会话持久化在本地数据库，面板重启后无需重新登录

## 目录与文件

| 路径 | 说明 |
| --- | --- |
| `/usr/local/bin/go-ui` | SSH 管理菜单（改密码/端口/路径、重启、日志、卸载） |
| `/opt/gost-webui/bin/gost-webui` | 面板主程序（内嵌前端） |
| `/opt/gost-webui/bin/gost` | gost 主程序（面板的子进程） |
| `/etc/gost-webui/panel.yml` | 面板配置（监听地址、管理员初始密码、gost API 凭据） |
| `/etc/gost-webui/gost.yml` | **面板自动生成**的 gost 配置（请勿手工修改） |
| `/var/lib/gost-webui/panel.db` | 面板数据库（节点、流量明细、设置） |
| `/var/lib/gost-webui/quota.json` | gost 配额持久化文件 |
| `/var/log/gost-webui/gost.log` | gost 运行日志 |

## 客户端的服务端配置要点

- 落地机上的入站**不需要**任何特殊设置，保持原本可用的配置即可
- 若落地机入站开启了「仅允许指定 IP」之类的白名单，请放行中转机 IP
- 若使用 TLS + 域名：客户端 SNI 仍是原域名，转发链路不受影响（中转机仅透传 TCP）
- 若落地机为 `hysteria2/tuic` 等 UDP 协议：请确认落地机防火墙已放行 UDP，且中转机同样放行 UDP

## 安全建议

1. 面板默认使用 HTTP，建议放在反向代理（Nginx/Caddy）后面启用 HTTPS，或仅通过内网/白名单访问面板端口
2. 首次登录后立即到「设置 → 修改密码」
3. `/etc/gost-webui/panel.yml` 权限为 600，内含面板初始密码与 gost API 密码，请勿泄露
4. 中转端口会对外开放，面板不会在端口上做鉴权；协议本身的加密与鉴权由落地机负责

## 开发与构建

```bash
# 依赖：Go >= 1.26
make build          # 编译面板 → bin/gost-webui
make gost           # 编译 gost（经加速镜像克隆源码）→ bin/gost
make test           # 单元测试（链接解析等）
make vet

# 本地调试
cp panel.example.yml panel.dev.yml   # 修改 bin 路径与端口后
./bin/gost-webui -c panel.dev.yml -listen :8787
```

项目结构：

```
gost-webui/
├── main.go                      # 入口：加载配置、启动 gost 守护、HTTP 服务
├── internal/
│   ├── config/                  # panel.yml 配置
│   ├── store/                   # bbolt 存储（节点、小时级流量明细、设置）
│   ├── model/                   # 节点/配额/限速等数据模型
│   ├── link/                    # v2rayN 链接解析与改写（含单元测试）
│   ├── gostmgr/                 # 生成 gost 配置、守护进程、REST API 客户端
│   ├── controller/              # 业务：应用节点到 gost、配额校正、流量采样
│   ├── alerts/                  # 事件检测与通知分发（上下线/流量/负载）
│   ├── notify/                  # Telegram Bot 通知（HTTP 推送）
│   ├── system/                  # 主机负载指标（CPU/内存/磁盘）
│   └── server/                  # 面板 HTTP API、鉴权、访问路径前缀
├── web/                         # 内嵌前端（原生 JS，无外链资源）
├── deploy/gost-webui.service    # systemd 单元
├── install.sh                   # 一键安装（支持源码/预编译包）
├── uninstall.sh                 # 一键卸载
└── release.sh                   # 打包发布（二进制 + 源码包）
```

## SSH 管理菜单 go-ui

安装后面板上任一台中转机，SSH 登录后直接输入：

```bash
go-ui            # 打开菜单
go-ui status     # 查看运行状态
go-ui passwd     # 修改管理员账号密码
go-ui port       # 修改面板端口
go-ui path       # 修改访问路径
go-ui restart    # 重启面板
go-ui logs       # 查看日志
go-ui uninstall  # 干净卸载
```

菜单支持（1~8）：查看运行状态、修改账号密码、修改端口、修改访问路径、修改服务器外网地址、
重启面板/gost、查看日志、干净卸载。修改配置时会自动停服务→写配置→重启，无需手工编辑文件。

> 面板同时支持命令行维护（go-ui 底层调用）：
> `gost-webui -c /etc/gost-webui/panel.yml -set-password "admin 新密码"`
> `-set-listen :8899` / `-set-base-path /panel` / `-set-public-host 1.2.3.4` / `-show`

## 常见问题

**Q：安装完成提示的地址是内网 IP（如 10.x / 192.168.x），浏览器打不开？**
A：面板监听的是 `0.0.0.0:端口`，能否外网访问取决于三件事：

1. **公网 IP**：云主机的内网 IP（10.x/172.16-31.x/192.168.x）不能从外网访问，请在云控制台查看公网 IP，
   或在服务器上执行 `curl -4 ip.sb` / `curl -4 ifconfig.me` 获取
2. **安全组**：云控制台 →「安全组/防火墙」→ 入站规则 → 放行面板端口 TCP（如 8787），
   同样要放行中转节点使用的端口（TCP 和 UDP 都放行）
3. **本机防火墙**：`ufw allow 8787/tcp`，或
   `firewall-cmd --add-port=8787/tcp --permanent && firewall-cmd --reload`

排查命令：

```bash
ss -lntp | grep 8787          # 应为 0.0.0.0:8787，若为 127.0.0.1:8787 说明配置里 listen 写死了回环
curl -4 ip.sb                 # 查看公网 IP
```

另外请到面板「设置 → 中转机地址」填写**公网 IP 或域名**（否则生成的中转链接会指向内网地址，客户端连不上）。
面板检测到内网地址时会在页面顶部给出警告。

**Q：面板显示「未检测到中转机地址」？**
A：面板通过公共接口探测公网 IP，若服务器出网受限会在设置里手动填写服务器 IP 或域名。

**Q：如何配置 Telegram 通知？**
A：
1. 在 Telegram 里找 `@BotFather`，发送 `/newbot` 创建机器人，得到 **Bot Token**（形如 `123456:ABC-DEF...`）
2. 给机器人发一条消息，或把机器人拉进群/频道；然后打开
   `https://api.telegram.org/bot<Token>/getUpdates` 找到 `chat.id`（群/频道为负数）
3. 面板「通知提醒」页填入 Token 与 Chat ID，按需勾选事件与阈值，点「发送测试」确认能收到
4. 若服务器无法直连 Telegram，把「API 地址」改成你的自建反代（例如 `https://tg.example.com`）

**Q：修改了面板端口/访问路径，为什么没生效？**
A：端口与访问路径需要重启面板（页面会提示并可直接点「重启」）。重启后请用新地址访问，
例如路径填 `/panel` 则访问 `http://IP:端口/panel/`；根路径会自动跳转过去。

**Q：通知太多了怎么办？**
A：调大「同类通知冷却」（默认 600 秒，同一事件在该时间内只发一次），
或只勾选需要的事件；流量预警按阈值档位触发，同一周期内每个档位只提醒一次。

**Q：添加节点报错「端口被占用」？**
A：换一个端口，或确认该端口未被其它程序占用（gost 需要绑定 TCP/UDP）。

**Q：客户端连不上，日志里能看到转发但无响应？**
A：检查落地机地址端口是否可达（可在中转机上 `curl -v telnet://落地IP:端口` 测试），
以及落地机防火墙是否放行了中转机 IP。

**Q：为什么「今日流量」和客户端统计对不上？**
A：中转机统计的是经过本机的字节数（含协议开销），且采样间隔内会有少量聚合误差，属正常现象。

**Q：gost 版本要求？**
A：面板依赖 gost 较新的 REST API（服务/配额热管理，`go-gost/x >= 0.17.2`）。
安装脚本会自动校验，不合格会回退到源码编译。

## License 与合规

- 本项目：MIT（见 [LICENSE](LICENSE)）
- 第三方组件：均为宽松型许可证（MIT / BSD-3-Clause / Apache-2.0），清单见
  [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md)
- gost 为 MIT，**不随本项目分发**（安装脚本在目标机器下载/编译）；若你要自行打包分发 gost 二进制，
  请一并附带其 MIT 许可证文本（Copyright (c) 2016 ginuerzh）
- 发布前建议保留 README 中的「免责声明与合规提示」与「与 gost 官方无关」的说明
