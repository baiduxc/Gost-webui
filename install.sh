#!/usr/bin/env bash
# GOST 面板 一键安装脚本
#
# 用法：
#   本地源码安装：  sudo bash install.sh
#   远程一键安装：  curl -fsSL <你的仓库地址>/raw/main/install.sh | sudo bash
#
# 可选环境变量：
#   PANEL_PORT=8787         面板端口
#   PANEL_DIR=/opt/gost-webui      安装目录
#   BASE_PATH=/panel        面板访问路径前缀（默认根路径）
#   PUBLIC_HOST=1.2.3.4     手动指定公网地址/域名（不填则自动探测）
#   PANEL_REPO=...         面板源码仓库（远程模式用）
#   GOPROXY=https://goproxy.cn,direct
#   GO_VERSION=1.26.8
#   GOST_REF=master         gost 源码分支/标签/提交
#   NO_SYSTEMD=1            不安装 systemd 服务（仅部署文件）

set -euo pipefail

PANEL_DIR="${PANEL_DIR:-/opt/gost-webui}"
CONF_DIR="${CONF_DIR:-/etc/gost-webui}"
DATA_DIR="${DATA_DIR:-/var/lib/gost-webui}"
LOG_DIR="${LOG_DIR:-/var/log/gost-webui}"
PANEL_PORT="${PANEL_PORT:-8787}"
BASE_PATH="${BASE_PATH:-}"
# 规范化访问路径：/panel
NORM_BASE=""
if [ -n "$BASE_PATH" ]; then
  NORM_BASE="/${BASE_PATH#/}"
  NORM_BASE="${NORM_BASE%/}"
fi
GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
# GitHub 加速域名（如 https://ghfast.top）。默认空=直连 GitHub；
# 交互式安装会询问 Y/n（默认 n=直连），也可用环境变量 GH_PROXY 或 --gh-proxy 预先指定。
GH_PROXY="${GH_PROXY:-}"
# 给 GitHub 地址加上加速前缀
gh_url() {
  local u="$1"
  if [ -n "$GH_PROXY" ]; then
    case "$u" in
      https://github.com/*|http://github.com/*) printf '%s/%s' "${GH_PROXY%/}" "$u" ;;
      *) printf '%s' "$u" ;;
    esac
  else
    printf '%s' "$u"
  fi
}
GOST_REPO="${GOST_REPO:-https://github.com/go-gost/gost}"
# sing-box（VLESS+REALITY 引擎）
SINGBOX_REPO="${SINGBOX_REPO:-https://github.com/SagerNet/sing-box}"
SINGBOX_VERSION="${SINGBOX_VERSION:-1.14.2}"
SINGBOX_BIN="${PANEL_DIR}/bin/sing-box"
GOST_REF="${GOST_REF:-master}"
GO_VERSION="${GO_VERSION:-1.26.8}"
# 发布前请把下面这行改成你自己的仓库地址（curl 一键安装时会从这里拉取源码）
DEFAULT_PANEL_REPO="https://github.com/baiduxc/gost-webui"
PANEL_REPO="${PANEL_REPO:-$DEFAULT_PANEL_REPO}"
# 预编译包地址（可选，支持 {arch} 占位符；设置后优先使用，安装更快且无需编译）
PANEL_RELEASE_URL="${PANEL_RELEASE_URL:-}"
# 自定义二进制（可选）：支持 http(s) 链接或本地文件路径；用于无法访问 GitHub 的服务器
GOST_BIN_SRC="${GOST_BIN_URL:-}"
PANEL_BIN_SRC="${PANEL_BIN_URL:-}"
ORG_NAME="GOST 面板"

RED=$'\033[31m'; GREEN=$'\033[32m'; YELLOW=$'\033[33m'; BLUE=$'\033[34m'; RESET=$'\033[0m'
info() { echo -e "${BLUE}[*]${RESET} $*"; }
ok()   { echo -e "${GREEN}[✓]${RESET} $*"; }
warn() { echo -e "${YELLOW}[!]${RESET} $*"; }
die()  { echo -e "${RED}[✗]${RESET} $*" >&2; exit 1; }

# GitHub 拉取失败时的提示
github_hint() {
  echo -e "${YELLOW}[!]${RESET} 无法从 GitHub 获取文件，请确认服务器网络可以访问 github.com。"
  echo "    替代方案：在能访问 GitHub 的机器上下载好二进制，上传到服务器后指定安装："
  echo "      sudo bash install.sh --gost-bin /root/gost --panel-bin /root/gost-webui"
  echo "    （参数也可以是任意可访问的下载地址，如 https://example.com/gost）"
}

[ "$(id -u)" = "0" ] || die "请使用 root 运行（sudo bash install.sh）"

# ---------- 基础环境 ----------
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) GOARCH=amd64 ;;
  aarch64|arm64) GOARCH=arm64 ;;
  armv7l) GOARCH=arm ;;
  *) die "不支持的架构: $ARCH" ;;
esac

if command -v apt-get >/dev/null 2>&1; then
  pkg_install() { DEBIAN_FRONTEND=noninteractive apt-get install -y -qq "$@" >/dev/null; }
  info "安装基础依赖 (apt)…"
  apt-get update -qq >/dev/null 2>&1 || true
  pkg_install ca-certificates curl git tar iproute2 || true
elif command -v yum >/dev/null 2>&1; then
  pkg_install() { yum install -y -q "$@" >/dev/null; }
  info "安装基础依赖 (yum)…"
  pkg_install ca-certificates curl git tar iproute || true
elif command -v apk >/dev/null 2>&1; then
  pkg_install() { apk add --no-cache "$@" >/dev/null; }
  pkg_install ca-certificates curl git tar iproute2 || true
else
  warn "未识别的包管理器，假定 curl/git/tar 已安装"
fi

for c in curl git tar; do
  command -v "$c" >/dev/null 2>&1 || die "缺少命令: $c"
done

# ---------- 安装 Go ----------
go_ok() {
  command -v go >/dev/null 2>&1 || return 1
  local v
  v="$(go version 2>/dev/null | awk '{print $3}' | sed 's/^go//')" || return 1
  [ -n "$v" ] || return 1
  # 需要 >= 1.24
  local major minor
  major="${v%%.*}"; minor="$(echo "$v" | cut -d. -f2)"
  [ "$major" -gt 1 ] || [ "$minor" -ge 24 ]
}

ensure_go() {
  if go_ok; then
    ok "已安装 Go: $(go version | awk '{print $3}')"
    return
  fi
  info "安装 Go ${GO_VERSION}（阿里云镜像）…"
  local url="https://mirrors.aliyun.com/golang/go${GO_VERSION}.linux-${GOARCH}.tar.gz"
  local tmp="/tmp/go${GO_VERSION}.tar.gz"
  curl -fsSL --retry 3 -o "$tmp" "$url" || die "下载 Go 失败: $url"
  rm -rf /usr/local/go
  tar -C /usr/local -xzf "$tmp"
  rm -f "$tmp"
  export PATH="/usr/local/go/bin:$PATH"
  go_ok || die "Go 安装失败"
  ok "Go 安装完成: $(go version | awk '{print $3}')"
}

export GOPROXY
export GOFLAGS="-mod=mod"
export CGO_ENABLED=0

# ---------- 工具函数 ----------
# 探测公网 IP（多线路，兼容出网受限环境）
detect_public_ip() {
  local ip url body
  for url in \
    "https://api.ipify.org" \
    "https://ipv4.icanhazip.com" \
    "https://ifconfig.me/ip" \
    "https://ip.sb" \
    "https://myip.ipip.net/s" \
    "https://www.cloudflare.com/cdn-cgi/trace" \
    "http://ip.3322.net"; do
    body="$(curl -fsS --max-time 6 --connect-timeout 4 "$url" 2>/dev/null || true)"
    [ -n "$body" ] || continue
    # cloudflare trace 需要提取 ip= 行
    ip="$(printf '%s\n' "$body" | sed -n 's/^ip=//p' | head -1 || true)"
    [ -n "$ip" ] || ip="$(printf '%s' "$body" | tr -d '\r' | head -1 | awk '{print $1}')"
    ip="$(printf '%s' "$ip" | tr -d '[:space:]')"
    if printf '%s' "$ip" | grep -qE '^([0-9]{1,3}\.){3}[0-9]{1,3}$|^[0-9a-fA-F:]{6,}$'; then
      echo "$ip"
      return 0
    fi
  done
  return 1
}

# 本机第一个非回环 IP（内网地址）
detect_local_ip() {
  hostname -I 2>/dev/null | tr ' ' '\n' | grep -vE '^(127\.|::1$|$)' | head -1 || true
}

# 放行本机防火墙端口
open_firewall_port() {
  local port="$1" opened=0
  if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -qi '^Status: active'; then
    if ufw allow "${port}/tcp" >/dev/null 2>&1; then
      ok "已在本机 ufw 放行 ${port}/tcp"
      opened=1
    fi
  fi
  if command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state >/dev/null 2>&1; then
    firewall-cmd --permanent --add-port="${port}/tcp" >/dev/null 2>&1 || true
    if firewall-cmd --reload >/dev/null 2>&1; then
      ok "已在本机 firewalld 放行 ${port}/tcp"
      opened=1
    fi
  fi
  # 极简环境：策略为 DROP/REJECT 且没有 ufw/firewalld 时补一条 iptables 规则
  if [ "$opened" = "0" ] && command -v iptables >/dev/null 2>&1; then
    if iptables -L INPUT -n 2>/dev/null | grep -qE 'DROP|REJECT'; then
      if iptables -I INPUT -p tcp --dport "$port" -j ACCEPT 2>/dev/null; then
        warn "已临时添加 iptables 放行规则（重启后失效，建议改用 ufw/firewalld 持久化）"
      fi
    fi
  fi
}
# 撤销本机防火墙端口规则（卸载时调用；规则不存在时静默跳过）
close_firewall_port() {
  local port="$1"
  [ -n "$port" ] || return 0
  if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -qi '^Status: active'; then
    ufw delete allow "${port}/tcp" >/dev/null 2>&1 && ok "已移除 ufw ${port}/tcp 规则" || true
  fi
  if command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state >/dev/null 2>&1; then
    firewall-cmd --permanent --remove-port="${port}/tcp" >/dev/null 2>&1 || true
    firewall-cmd --reload >/dev/null 2>&1 && ok "已移除 firewalld ${port}/tcp 规则" || true
  fi
  if command -v iptables >/dev/null 2>&1; then
    while iptables -D INPUT -p tcp --dport "$port" -j ACCEPT >/dev/null 2>&1; do :; done
  fi
}

# 验证 gost 是否具备面板所需的 REST API（服务/配额热管理）
verify_gost() {
  local bin="$1" tmpdir port pid
  [ -x "$bin" ] || return 1
  "$bin" -V >/dev/null 2>&1 || return 1
  tmpdir="$(mktemp -d)"
  port=$((20000 + RANDOM % 20000))
  cat > "$tmpdir/gost.yml" <<EOF
services: []
api:
  addr: "127.0.0.1:$port"
  auth:
    username: t
    password: t
log:
  level: error
EOF
  "$bin" -C "$tmpdir/gost.yml" >/dev/null 2>&1 &
  pid=$!
  local i code="000"
  for i in $(seq 1 20); do
    sleep 0.3
    code="$(curl -s -o /dev/null -w '%{http_code}' -u t:t "http://127.0.0.1:$port/config/services" 2>/dev/null || echo 000)"
    [ "$code" = "200" ] && break
  done
  local qcode="000"
  qcode="$(curl -s -o /dev/null -w '%{http_code}' -u t:t "http://127.0.0.1:$port/config/quotas" 2>/dev/null || echo 000)"
  kill "$pid" >/dev/null 2>&1 || true
  wait "$pid" 2>/dev/null || true
  rm -rf "$tmpdir"
  [ "$code" = "200" ] && [ "$qcode" = "200" ]
}

build_gost() {
  local work; work="$(mktemp -d)"
  info "拉取 gost 源码…"
  local repo; repo="$(gh_url "$GOST_REPO")"
  if ! git clone --depth 1 --branch "$GOST_REF" "$repo" "$work/gost" >/dev/null 2>&1; then
    # 分支可能不存在（如提交号），退化为默认分支
    if ! git clone --depth 1 "$repo" "$work/gost" >/dev/null 2>&1; then
      github_hint
      die "克隆 gost 源码失败"
    fi
  fi
  info "编译 gost（首次编译约 3~10 分钟，请耐心等待）…"
  ( cd "$work/gost" && go build -ldflags="-s -w" -o "$PANEL_DIR/bin/gost" ./cmd/gost ) || die "编译 gost 失败"
  rm -rf "$work"
  ok "gost 编译完成"
}

install_gost() {
  mkdir -p "$PANEL_DIR/bin"
  if [ -n "$GOST_BIN_SRC" ]; then
    info "使用指定的 gost 二进制: $GOST_BIN_SRC"
    fetch_bin "$GOST_BIN_SRC" "$PANEL_DIR/bin/gost" || die "获取 gost 二进制失败: $GOST_BIN_SRC"
    verify_gost "$PANEL_DIR/bin/gost" || die "指定的 gost 二进制不可用（需要较新版本，支持服务/配额 REST API）"
    ok "gost 已就绪"
    return
  fi
  # 1) 已有可用二进制则复用
  if [ -x "$PANEL_DIR/bin/gost" ] && verify_gost "$PANEL_DIR/bin/gost"; then
    ok "复用已安装的 gost"
    return
  fi
  # 2) 尝试 GitHub Release 预编译包（快速）
  if try_release_gost; then
    return
  fi
  # 3) 从源码编译（可靠）
  ensure_go
  build_gost
  verify_gost "$PANEL_DIR/bin/gost" || die "编译出的 gost 不支持所需 API，请检查 GOST_REF"
}

try_release_gost() {
  local ver rurl tmp n
  for ver in 3.3.0 3.2.6 3.2.5; do
    rurl="$(gh_url "https://github.com/go-gost/gost/releases/download/v${ver}/gost_${ver}_linux_${GOARCH}.tar.gz")"
    [ "$GOARCH" = "arm" ] && rurl="$(gh_url "https://github.com/go-gost/gost/releases/download/v${ver}/gost_${ver}_linux_armv7.tar.gz")"
    tmp="/tmp/gost_${ver}.tar.gz"
    if [ -n "$GH_PROXY" ]; then
      info "下载 gost v${ver} 预编译包（经加速节点 ${GH_PROXY}）…"
    else
      info "下载 gost v${ver} 预编译包（直连 GitHub）…"
    fi
    echo "    $rurl"
    if curl -fSL --progress-bar --retry 2 --connect-timeout 15 -o "$tmp" "$rurl"; then
      n=$(du -h "$tmp" 2>/dev/null | cut -f1)
      ok "下载完成（$n），校验中…"
      if tar -xzf "$tmp" -C /tmp gost 2>/dev/null; then
        mv /tmp/gost "$PANEL_DIR/bin/gost"
        chmod +x "$PANEL_DIR/bin/gost"
        if verify_gost "$PANEL_DIR/bin/gost"; then
          ok "gost v${ver} 预编译包可用"
          rm -f "$tmp"
          return 0
        fi
      fi
    fi
    rm -f "$tmp"
  done
  warn "未获取到可用的 gost 预编译包（下载失败或版本不兼容），改为源码编译"
  github_hint
  return 1
}

# ---------- 安装 sing-box（VLESS+REALITY 引擎，失败不阻塞） ----------
install_singbox() {
  local ver url tmp
  ver="$SINGBOX_VERSION"
  tmp="/tmp/sing-box_${ver}.tar.gz"
  url="$(gh_url "https://github.com/SagerNet/sing-box/releases/download/v${ver}/sing-box-${ver}-linux-${GOARCH}.tar.gz")"
  info "下载 sing-box v${ver}（VLESS+REALITY 引擎）…"
  echo "    $url"
  if curl -fSL --progress-bar --retry 2 --connect-timeout 15 -o "$tmp" "$url"; then
    if tar -xzf "$tmp" -C /tmp "sing-box-${ver}-linux-${GOARCH}/sing-box" 2>/dev/null; then
      mv "/tmp/sing-box-${ver}-linux-${GOARCH}/sing-box" "$SINGBOX_BIN"
      chmod +x "$SINGBOX_BIN"
      rm -f "$tmp" "/tmp/sing-box-${ver}-linux-${GOARCH}" -r
      ok "sing-box 安装完成：$SINGBOX_BIN"
      return 0
    fi
  fi
  rm -f "$tmp"
  warn "sing-box 下载失败，VLESS+REALITY 节点暂不可用；可稍后手动安装到 $SINGBOX_BIN"
  return 0
}

# ---------- 安装面板 ----------
build_panel() {
  local src="$1"
  info "编译面板…"
  ( cd "$src" && go build -ldflags="-s -w" -o "$PANEL_DIR/bin/gost-webui" . ) || die "编译面板失败"
  ok "面板编译完成"
}

# 下载预编译面板（PANEL_RELEASE_URL，支持 {arch} 占位符）
try_release_panel() {
  [ -n "$PANEL_RELEASE_URL" ] || return 1
  local url tmp dir
  url="${PANEL_RELEASE_URL//\{arch\}/$GOARCH}"
  url="${url//\{os\}/linux}"
  tmp="/tmp/gost-webui_release.tar.gz"
  info "下载预编译面板包…"
  if ! curl -fsSL --retry 3 --connect-timeout 20 -o "$tmp" "$url" 2>/dev/null; then
    warn "面板预编译包下载失败：$url"
    return 1
  fi
  dir="$(mktemp -d)"
  if ! tar -xzf "$tmp" -C "$dir" gost-webui 2>/dev/null; then
    warn "预编译包解压失败"
    rm -rf "$dir" "$tmp"
    return 1
  fi
  mkdir -p "$PANEL_DIR/bin" "$PANEL_DIR/web" "$PANEL_DIR/deploy"
  mv "$dir/gost-webui" "$PANEL_DIR/bin/gost-webui"
  chmod +x "$PANEL_DIR/bin/gost-webui"
  rm -rf "$dir" "$tmp"
  ok "已安装预编译面板: $("$PANEL_DIR/bin/gost-webui" -v 2>/dev/null || echo gost-webui)"
  return 0
}

install_panel() {
  if [ -n "$PANEL_BIN_SRC" ]; then
    info "使用指定的面板二进制: $PANEL_BIN_SRC"
    mkdir -p "$PANEL_DIR/bin"
    if fetch_bin "$PANEL_BIN_SRC" "$PANEL_DIR/bin/gost-webui" && "$PANEL_DIR/bin/gost-webui" -v >/dev/null 2>&1; then
      ok "面板二进制已安装: $("$PANEL_DIR/bin/gost-webui" -v)"
      return 0
    fi
    warn "指定的面板二进制不可用，继续其它安装方式"
  fi
  if try_release_panel; then
    return 0
  fi
  local src="" script_dir
  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  if [ -f "$script_dir/go.mod" ] && [ -d "$script_dir/web" ]; then
    src="$script_dir"
    info "使用本地源码目录: $src"
  else
    src="$(mktemp -d)/gost-webui-src"
    local repo="${PANEL_REPO:-}"
    if [ -z "$repo" ]; then
      die "未找到本地源码，且仓库地址未配置。
    请使用环境变量指定仓库：sudo PANEL_REPO=https://github.com/you/gost-webui bash install.sh"
    fi
    repo="$(gh_url "$repo")"
    info "拉取面板源码: $repo"
    if ! git clone --depth 1 "$repo" "$src" >/dev/null 2>&1; then
      github_hint
      die "拉取面板源码失败"
    fi
  fi
  ensure_go
  build_panel "$src"
  # 拷贝前端与文档，便于排障
  mkdir -p "$PANEL_DIR/web" "$PANEL_DIR/deploy"
  cp -r "$src/web/." "$PANEL_DIR/web/" 2>/dev/null || true
  cp "$src/install.sh" "$PANEL_DIR/" 2>/dev/null || true
  cp "$src/deploy/gost-webui.service" "$PANEL_DIR/deploy/" 2>/dev/null || true
}

# 获取二进制：src 可以是 http(s) 链接，也可以是本地文件路径
fetch_bin() {
  local src="$1" dst="$2"
  case "$src" in
    http://*|https://*)
      curl -fsSL --retry 2 --connect-timeout 20 -o "$dst" "$src" || return 1
      ;;
    *)
      [ -f "$src" ] || return 1
      cp -f "$src" "$dst" || return 1
      ;;
  esac
  chmod +x "$dst" 2>/dev/null || true
  return 0
}

# 选择一个空闲的本地端口（用于 gost API）
# ---------- 命令行参数与交互式配置 ----------
PANEL_USER_IN="${PANEL_USER:-}"
PANEL_PASS_IN="${PANEL_PASS:-}"

usage() {
  cat <<EOF
用法: bash install.sh [选项]

  -p, --port <端口>      面板端口（默认 8787）
      --path <路径>      访问路径前缀，如 /panel（默认根路径）
  -u, --user <用户名>    管理员用户名（默认 admin）
  -P, --pass <密码>      管理员密码（默认随机生成）
      --gost-bin <路径|URL>  指定 gost 二进制（无法访问 GitHub 时使用）
      --panel-bin <路径|URL> 指定面板二进制（无法访问 GitHub 时使用）
      --gh-proxy <URL>   GitHub 加速代理域名（如 https://ghfast.top），默认直连
      --uninstall        干净卸载（服务、程序、配置、数据）
  -h, --help             显示帮助

不带参数且为交互式终端时，脚本会依次询问端口/路径/用户名/密码。
也可用环境变量：PANEL_PORT、BASE_PATH、PANEL_USER、PANEL_PASS、PUBLIC_HOST、
PANEL_REPO、PANEL_RELEASE_URL、GOST_BIN_URL、PANEL_BIN_URL、GH_PROXY、GOPROXY、NO_SYSTEMD
EOF
}

parse_args() {
  while [ $# -gt 0 ]; do
    case "$1" in
      -p|--port) PANEL_PORT="${2:-}"; shift 2 2>/dev/null || shift ;;
      --path) BASE_PATH="${2:-}"; shift 2 2>/dev/null || shift ;;
      -u|--user) PANEL_USER_IN="${2:-}"; shift 2 2>/dev/null || shift ;;
      -P|--pass) PANEL_PASS_IN="${2:-}"; shift 2 2>/dev/null || shift ;;
      --gost-bin|--gost-bin-url) GOST_BIN_SRC="${2:-}"; shift 2 2>/dev/null || shift ;;
      --panel-bin|--panel-bin-url) PANEL_BIN_SRC="${2:-}"; shift 2 2>/dev/null || shift ;;
      --gh-proxy) GH_PROXY="${2:-}"; shift 2 2>/dev/null || shift ;;
      --uninstall) uninstall_all; exit 0 ;;
      -h|--help) usage; exit 0 ;;
      *) shift ;;
    esac
  done
  case "$PANEL_PORT" in
    ''|*[!0-9]*) die "端口必须是数字: $PANEL_PORT" ;;
  esac
  if [ "$PANEL_PORT" -lt 1 ] || [ "$PANEL_PORT" -gt 65535 ]; then
    die "端口范围应为 1-65535"
  fi
  if [ -n "$BASE_PATH" ]; then
    NORM_BASE="/${BASE_PATH#/}"
    NORM_BASE="${NORM_BASE%/}"
  fi
}

# 交互式询问初始配置（stdin 被管道占用时改从 /dev/tty 读取，真无终端才跳过）
interactive_config() {
  [ -f "$CONF_DIR/panel.yml" ] && return 0
  local TTY_IN=""
  if [ -t 0 ]; then
    TTY_IN="/dev/stdin"
  elif [ -e /dev/tty ] && (exec < /dev/tty) 2>/dev/null; then
    TTY_IN="/dev/tty"
  else
    return 0
  fi
  echo
  echo "---------- 面板初始配置（直接回车使用默认值）----------"
  local v
  if [ -z "$GH_PROXY" ]; then
    read -rp "是否使用 GitHub 加速节点 (ghfast.top) 下载？[y/N]: " v < "$TTY_IN" || true
    case "${v:-n}" in
      [Yy]*) GH_PROXY="https://ghfast.top" ;;
      *) GH_PROXY="" ;;
    esac
    if [ -n "$GH_PROXY" ]; then
      echo "  将通过加速节点下载: $GH_PROXY"
    else
      echo "  直连 GitHub 下载"
    fi
  fi
  read -rp "面板端口 [${PANEL_PORT}]: " v < "$TTY_IN" || true
  if [ -n "$v" ]; then PANEL_PORT="$v"; fi
  read -rp "访问路径（如 /panel，留空为根路径）[${NORM_BASE:-/}]: " v < "$TTY_IN" || true
  if [ -n "$v" ]; then
    BASE_PATH="$v"
    NORM_BASE="/${BASE_PATH#/}"
    NORM_BASE="${NORM_BASE%/}"
  fi
  read -rp "管理员用户名 [${PANEL_USER_IN:-admin}]: " v < "$TTY_IN" || true
  if [ -n "$v" ]; then PANEL_USER_IN="$v"; fi
  read -rsp "管理员密码（留空随机生成）: " v < "$TTY_IN" || true
  echo
  if [ -n "$v" ]; then PANEL_PASS_IN="$v"; fi
  echo "------------------------------------------------------"
  echo
}

pick_free_port() {
  local p ports
  ports="18080 18081 18082 18083 18084 28080"
  for p in $ports; do
    if ! (ss -lnt 2>/dev/null || netstat -lnt 2>/dev/null) | awk '{print $4}' | grep -qE "[:.]${p}$"; then
      echo "$p"
      return 0
    fi
  done
  echo $((20000 + RANDOM % 20000))
}

# ---------- 配置 ----------
write_config() {
  mkdir -p "$CONF_DIR" "$DATA_DIR" "$LOG_DIR" "$PANEL_DIR/bin"
  if [ -f "$CONF_DIR/panel.yml" ]; then
    ok "配置文件已存在，保留原配置: $CONF_DIR/panel.yml"
    local saved_listen saved_base saved_host saved_user
    saved_listen="$(grep -E '^listen:' "$CONF_DIR/panel.yml" | head -1 | cut -d: -f2- | tr -d '"'"'"'[:space:]' || true)"
    saved_base="$(grep -E '^base_path:' "$CONF_DIR/panel.yml" | head -1 | cut -d: -f2- | tr -d '"'"'"'[:space:]' || true)"
    saved_host="$(grep -E '^public_host:' "$CONF_DIR/panel.yml" | head -1 | cut -d: -f2- | tr -d '"'"'"'[:space:]' || true)"
    saved_user="$(awk '/^admin:/{f=1;next} f&&/^[^[:space:]]/{f=0} f' "$CONF_DIR/panel.yml" | grep -E '^[[:space:]]+username:' | head -1 | cut -d: -f2- | tr -d '"'"'"'[:space:]' || true)"
    if [ -n "$saved_listen" ]; then
      local saved_port="${saved_listen##*:}"
      case "$saved_port" in ''|*[!0-9]*) ;; *) PANEL_PORT="$saved_port" ;; esac
    fi
    NORM_BASE="$saved_base"
    [ -n "$saved_host" ] && PUBLIC_HOST="$saved_host"
    PANEL_USER="${saved_user:-admin}"
    PANEL_PASS="$(grep -E '^[[:space:]]+password:' "$CONF_DIR/panel.yml" | head -1 | sed -E 's/.*password:[[:space:]]*//' | tr -d '"'"'"'' || true)"
    return
  fi
  PANEL_USER="${PANEL_USER_IN:-admin}"
  local api_port
  api_port="$(pick_free_port)"
  if [ -n "${PANEL_PASS_IN:-}" ]; then
    PANEL_PASS="$PANEL_PASS_IN"
  else
    PANEL_PASS="$(head -c 9 /dev/urandom | base64 | tr -d '=+/' | head -c 12)"
  fi
  GOST_API_PASS="$(head -c 24 /dev/urandom | base64 | tr -d '=+/' | head -c 24)"
  # 提前探测公网 IP，写入配置，避免生成的客户端链接指向内网地址
  local pub="${PUBLIC_HOST:-}"
  if [ -z "$pub" ]; then
    pub="$(detect_public_ip || true)"
  fi
  cat > "$CONF_DIR/panel.yml" <<EOF
# GOST 面板配置
listen: ":${PANEL_PORT}"
base_path: "${NORM_BASE}"
data_dir: "${DATA_DIR}"
log_dir: "${LOG_DIR}"
# 服务器对外地址（生成客户端链接用）；留空则面板自动探测
public_host: "${pub}"
sample_seconds: 15
retention_days: 90

admin:
  username: "${PANEL_USER}"
  password: "${PANEL_PASS}"

# VLESS+REALITY 引擎（可选；二进制存在即启用）
singbox:
  bin: "${PANEL_DIR}/bin/sing-box"
  dir: "${PANEL_DIR}/singbox"

gost:
  bin: "${PANEL_DIR}/bin/gost"
  config_file: "${CONF_DIR}/gost.yml"
  api_addr: "127.0.0.1:${api_port}"
  api_username: "gost"
  api_password: "${GOST_API_PASS}"
  log_file: "${LOG_DIR}/gost.log"
EOF
  chmod 600 "$CONF_DIR/panel.yml"
  if [ -n "$pub" ]; then
    ok "已生成配置（公网地址: ${pub}）: $CONF_DIR/panel.yml"
  else
    warn "未探测到公网 IP，可登录面板后在「设置」中手动填写"
    ok "已生成配置: $CONF_DIR/panel.yml"
  fi

  # 预置一个空的 gost 配置，避免首次启动失败
  cat > "$CONF_DIR/gost.yml" <<EOF
services: []
api:
  addr: "127.0.0.1:${api_port}"
  auth:
    username: "gost"
    password: "${GOST_API_PASS}"
log:
  level: info
  output: stderr
EOF
  chmod 600 "$CONF_DIR/gost.yml"
}

install_goui() {
  cat > /usr/local/bin/go-ui <<'GOUI_SCRIPT'
#!/usr/bin/env bash
# go-ui —— GOST 面板管理菜单（SSH 登录后直接输入 go-ui 即可）
set -uo pipefail

PANEL_DIR="/opt/gost-webui"
CONF_DIR="/etc/gost-webui"
DATA_DIR="/var/lib/gost-webui"
LOG_DIR="/var/log/gost-webui"
CONF="$CONF_DIR/panel.yml"
BIN="$PANEL_DIR/bin/gost-webui"
SERVICE="gost-webui"
UNIT="/etc/systemd/system/gost-webui.service"

B=$'\033[1m'; G=$'\033[32m'; R=$'\033[31m'; Y=$'\033[33m'; N=$'\033[0m'
line() { printf '%s\n' "------------------------------------------------------------"; }
ok()   { printf '%s✔%s %s\n' "$G" "$N" "$*"; }
err()  { printf '%s✘%s %s\n' "$R" "$N" "$*"; }
warn() { printf '%s!%s %s\n' "$Y" "$N" "$*"; }
pause(){ printf '\n按回车返回菜单…'; read -r _ || true; }
need_root() { [ "$(id -u)" = 0 ] || { err "请用 root 运行：sudo go-ui"; exit 1; }; }

get_top()  { grep -E "^$1:" "$CONF" 2>/dev/null | head -1 | sed -E "s/^$1:[[:space:]]*//" | tr -d '"'; }
get_admin(){ awk '/^admin:/{f=1;next} f&&/^[^[:space:]]/{f=0} f' "$CONF" 2>/dev/null | grep -E "^[[:space:]]+$1:" | head -1 | sed -E "s/^[[:space:]]*$1:[[:space:]]*//" | tr -d '"'; }
is_active(){ systemctl is-active --quiet "$SERVICE" 2>/dev/null; }

stop_svc()  { systemctl stop "$SERVICE" >/dev/null 2>&1 || true; sleep 1; }
start_svc() { systemctl start "$SERVICE" >/dev/null 2>&1 || true; sleep 2; }

restart_svc() {
  systemctl restart "$SERVICE" >/dev/null 2>&1 || true
  sleep 3
  if is_active; then ok "面板已重启"; else err "面板未启动，请查看日志（菜单 7）"; fi
}

show_status() {
  clear; header "运行状态"
  local listen base host user
  listen="$(get_top listen)"; base="$(get_top base_path)"; host="$(get_top public_host)"; user="$(get_admin username)"
  printf '  服务状态 : %s\n' "$(is_active && echo "${G}运行中${N}" || echo "${R}未运行${N}")"
  printf '  监听地址 : %s\n' "${listen:-未知}"
  printf '  访问路径 : %s\n' "${base:-/}"
  printf '  服务器地址: %s\n' "${host:-（自动探测）}"
  printf '  管理员   : %s\n' "${user:-admin}"
  printf '  程序版本 : %s\n' "$("$BIN" -v 2>/dev/null || echo 未知)"
  printf '  gost 进程: %s\n' "$(pgrep -f "$PANEL_DIR/bin/gost -C" >/dev/null && echo "运行中" || echo "未运行")"
  echo
  printf '  面板端口监听:\n'
  (ss -lntp 2>/dev/null || netstat -lntp 2>/dev/null) | grep -E ":${listen##*:} " | sed 's/^/    /' || echo "    （未监听）"
  pause
}

change_account() {
  clear; header "修改管理员账号密码"
  local u p1 p2
  read -rp "  新用户名 [$(get_admin username || echo admin)]: " u || true
  u="${u:-$(get_admin username || echo admin)}"
  read -rsp "  新密码（至少 6 位）: " p1 || true; echo
  read -rsp "  确认密码: " p2 || true; echo
  [ -n "$p1" ] || { err "密码不能为空"; pause; return; }
  [ "$p1" = "$p2" ] || { err "两次输入不一致"; pause; return; }
  [ ${#p1} -ge 6 ] || { err "密码至少 6 位"; pause; return; }
  stop_svc
  if "$BIN" -c "$CONF" -set-password "$u $p1"; then ok "账号已更新，正在重启面板…"; else err "更新失败"; fi
  start_svc
  is_active && ok "完成，请用新账号登录" || err "面板启动异常"
  pause
}

change_port() {
  clear; header "修改面板端口"
  local cur p
  cur="$(get_top listen)"; cur="${cur##*:}"
  read -rp "  新端口 [当前 ${cur:-8787}]: " p || true
  [ -n "$p" ] || { warn "未修改"; pause; return; }
  case "$p" in *[!0-9]*) err "端口必须是数字"; pause; return ;; esac
  [ "$p" -ge 1 ] && [ "$p" -le 65535 ] || { err "端口范围 1-65535"; pause; return; }
  if (ss -lnt 2>/dev/null || netstat -lnt 2>/dev/null) | awk '{print $4}' | grep -qE "[:.]${p}$"; then
    err "端口 ${p} 已被占用"; pause; return
  fi
  stop_svc
  "$BIN" -c "$CONF" -set-listen ":$p" && ok "端口已改为 $p"
  start_svc
  is_active && ok "面板已在新端口启动" || err "启动失败，请用旧端口检查日志"
  pause
}

change_path() {
  clear; header "修改访问路径"
  local cur p
  cur="$(get_top base_path)"
  read -rp "  新路径（如 /panel，留空为根路径）[当前 ${cur:-/}]: " p || true
  stop_svc
  "$BIN" -c "$CONF" -set-base-path "$p" && ok "访问路径已更新"
  start_svc
  is_active && ok "完成，请用新路径访问" || err "启动失败"
  pause
}

change_host() {
  clear; header "修改服务器外网地址"
  warn "用于生成客户端链接；云服务器请填公网 IP 或域名"
  local cur p
  cur="$(get_top public_host)"
  read -rp "  新地址（留空=自动探测）[当前 ${cur:-自动}] : " p || true
  stop_svc
  "$BIN" -c "$CONF" -set-public-host "$p" && ok "地址已更新"
  start_svc
  is_active && ok "完成" || err "启动失败"
  pause
}

do_restart() {
  clear; header "重启"
  printf '  1) 重启面板（含 gost）\n  2) 仅重启 gost\n  0) 返回\n'
  read -rp "  请选择: " c || true
  case "$c" in
    1) restart_svc ;;
    2) pkill -f "$PANEL_DIR/bin/gost -C" 2>/dev/null && ok "gost 已被守护进程自动拉起" || warn "未找到 gost 进程" ;;
    *) return ;;
  esac
  pause
}

show_logs() {
  clear; header "日志"
  printf '  1) 面板日志（journalctl）\n  2) gost 日志\n  0) 返回\n'
  read -rp "  请选择: " c || true
  case "$c" in
    1) journalctl -u "$SERVICE" -n 80 --no-pager 2>/dev/null | tail -80 ;;
    2) tail -n 80 "$LOG_DIR/gost.log" 2>/dev/null || warn "日志文件不存在" ;;
    *) return ;;
  esac
  pause
}

do_uninstall() {
  clear; header "干净卸载"
  warn "将删除：systemd 服务、程序 ${PANEL_DIR}、配置 ${CONF_DIR}"
  read -rp "  是否保留数据与日志（${DATA_DIR}）? [y/N]: " keep || true
  read -rp "  确认卸载？输入 yes 继续: " c || true
  [ "$c" = "yes" ] || { warn "已取消"; pause; return; }
  systemctl stop "$SERVICE" >/dev/null 2>&1 || true
  systemctl disable "$SERVICE" >/dev/null 2>&1 || true
  rm -f "$UNIT"; systemctl daemon-reload >/dev/null 2>&1 || true
  pkill -f "$PANEL_DIR/bin/gost-webui" >/dev/null 2>&1 || true
  pkill -f "$PANEL_DIR/bin/gost -C" >/dev/null 2>&1 || true
  sleep 1
  if [ -f "$PANEL_DIR/uninstall.sh" ]; then
    KEEP_DATA="$([ "${keep,,}" = "y" ] && echo 1 || echo 0)" bash "$PANEL_DIR/uninstall.sh" >/dev/null 2>&1 || true
  fi
  rm -rf "$PANEL_DIR" "$CONF_DIR"
  [ "${keep,,}" = "y" ] || { rm -rf "$DATA_DIR" "$LOG_DIR" "${HOME}/.gost" /root/.gost; rm -f /tmp/gost_install.log; }
  # 撤销安装时放行的端口规则
  pport=$(awk -F: '/^listen:/{gsub(/[ "]/,"",$2); print $2}' "$CONF" 2>/dev/null | tail -1)
  if [ -n "$pport" ] && command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -qi '^Status: active'; then
    ufw delete allow "${pport}/tcp" >/dev/null 2>&1 || true
  fi
  if command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state >/dev/null 2>&1 && [ -n "$pport" ]; then
    firewall-cmd --permanent --remove-port="${pport}/tcp" >/dev/null 2>&1 || true
    firewall-cmd --reload >/dev/null 2>&1 || true
  fi
  if command -v iptables >/dev/null 2>&1 && [ -n "$pport" ]; then
    while iptables -D INPUT -p tcp --dport "$pport" -j ACCEPT >/dev/null 2>&1; do :; done
  fi
  rm -f /usr/local/bin/go-ui
  ok "卸载完成，再见"
  exit 0
}

header() {
  clear 2>/dev/null || true
  printf '%s\n' "============================================================"
  printf '%s   %s%s\n' "" "$1" "$N"
  printf '%s\n' "============================================================"
}

menu() {
  header "GOST 面板管理菜单   ($(hostname 2>/dev/null))"
  printf '  服务状态: %s\n' "$(is_active && echo "${G}运行中${N}" || echo "${R}未运行${N}")"
  line
  printf '  1) 查看运行状态\n'
  printf '  2) 修改管理员账号密码\n'
  printf '  3) 修改面板端口\n'
  printf '  4) 修改访问路径\n'
  printf '  5) 修改服务器外网地址\n'
  printf '  6) 重启面板 / gost\n'
  printf '  7) 查看日志\n'
  printf '  8) 干净卸载\n'
  printf '  0) 退出\n'
  line
  read -rp "请选择 [0-8]: " c || exit 0
  case "$c" in
    1) show_status; menu ;;
    2) change_account; menu ;;
    3) change_port; menu ;;
    4) change_path; menu ;;
    5) change_host; menu ;;
    6) do_restart; menu ;;
    7) show_logs; menu ;;
    8) do_uninstall ;;
    0|q|"") echo "再见"; exit 0 ;;
    *) menu ;;
  esac
}

need_root
[ -f "$CONF" ] || { err "未找到配置文件 $CONF，请先安装"; exit 1; }

case "${1:-}" in
  status)  show_status ;;
  passwd)  change_account ;;
  port)    change_port ;;
  path)    change_path ;;
  restart) restart_svc ;;
  logs)    show_logs ;;
  uninstall) do_uninstall ;;
  *)       menu ;;
esac
GOUI_SCRIPT
  chmod +x /usr/local/bin/go-ui
  # 自定义目录时替换脚本内的路径
  if [ "$PANEL_DIR" != "/opt/gost-webui" ] || [ "$CONF_DIR" != "/etc/gost-webui" ] ||
     [ "$DATA_DIR" != "/var/lib/gost-webui" ] || [ "$LOG_DIR" != "/var/log/gost-webui" ]; then
    sed -i "s#^PANEL_DIR=.*#PANEL_DIR=\"$PANEL_DIR\"#; s#^CONF_DIR=.*#CONF_DIR=\"$CONF_DIR\"#; s#^DATA_DIR=.*#DATA_DIR=\"$DATA_DIR\"#; s#^LOG_DIR=.*#LOG_DIR=\"$LOG_DIR\"#" /usr/local/bin/go-ui
  fi
  ok "已安装管理菜单命令：go-ui"
}

install_systemd() {
  [ "${NO_SYSTEMD:-0}" = "1" ] && { warn "已跳过 systemd 安装（NO_SYSTEMD=1）"; return; }
  command -v systemctl >/dev/null 2>&1 || { warn "未检测到 systemd，请手动启动面板"; return; }
  cat > /etc/systemd/system/gost-webui.service <<EOF
[Unit]
Description=${ORG_NAME} (gost relay panel)
Documentation=https://github.com/go-gost/gost
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=${PANEL_DIR}
ExecStart=${PANEL_DIR}/bin/gost-webui -c ${CONF_DIR}/panel.yml
Restart=always
RestartSec=3
LimitNOFILE=1048576
KillMode=control-group
TimeoutStopSec=15

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable gost-webui >/dev/null 2>&1 || true
  systemctl restart gost-webui >/dev/null 2>&1 || true
  sleep 2
  if systemctl is-active --quiet gost-webui; then
    ok "systemd 服务已启动: gost-webui"
  else
    warn "服务未正常启动，最近日志如下："
    journalctl -u gost-webui -n 50 --no-pager 2>/dev/null || true
  fi
}

print_summary() {
  local pub local_ip
  pub="${PUBLIC_HOST:-}"
  if [ -z "$pub" ]; then
    pub="$(detect_public_ip || true)"
  fi
  local_ip="$(detect_local_ip || true)"

  echo
  echo "=================================================================="
  echo -e "  ${GREEN}${ORG_NAME} 安装完成${RESET}"
  echo "------------------------------------------------------------------"
  if [ -n "$pub" ]; then
    echo -e "  面板地址 : http://${pub}:${PANEL_PORT}${NORM_BASE}/     ${GREEN}(公网)${RESET}"
    [ -n "$local_ip" ] && [ "$local_ip" != "$pub" ] && \
      echo "  内网地址 : http://${local_ip}:${PANEL_PORT}${NORM_BASE}/"
  else
    echo -e "  面板地址 : http://<你的服务器公网IP>:${PANEL_PORT}${NORM_BASE}/"
    warn "未能自动探测到公网 IP（可能是 NAT/出网受限环境）"
    [ -n "$local_ip" ] && echo "  本机内网 IP : ${local_ip}（外网无法直接访问）"
  fi
  echo "  用户名   : ${PANEL_USER:-admin}"
  echo "  登录密码 : ${PANEL_PASS:-（见 ${CONF_DIR}/panel.yml）}"
  echo "  访问路径 : ${NORM_BASE:-/}（可在面板「系统设置」中修改）"
  echo "  配置文件 : ${CONF_DIR}/panel.yml"
  echo "  数据目录 : ${DATA_DIR}"
  echo "  日志     : ${LOG_DIR}"
  echo "------------------------------------------------------------------"
  echo "  常用命令 :"
  echo "    管理菜单  go-ui        （改密码/端口/路径、重启、卸载）"
  echo "    查看状态  systemctl status gost-webui"
  echo "    查看日志  journalctl -u gost-webui -f"
  echo "    重启服务  systemctl restart gost-webui"
  echo "    卸载      bash ${PANEL_DIR}/install.sh --uninstall"
  echo "=================================================================="
  echo
}

uninstall_all() {
  warn "开始卸载 ${ORG_NAME}…"
  if command -v systemctl >/dev/null 2>&1; then
    systemctl stop gost-webui >/dev/null 2>&1 || true
    systemctl disable gost-webui >/dev/null 2>&1 || true
    rm -f /etc/systemd/system/gost-webui.service
    systemctl daemon-reload || true
  fi
  pkill -f "${PANEL_DIR}/bin/gost-webui" >/dev/null 2>&1 || true
  pkill -f "${PANEL_DIR}/bin/gost" >/dev/null 2>&1 || true
  sleep 1
  # go-ui 管理命令一并移除（旧版 --uninstall 漏删，属卸载不干净 BUG）
  rm -f /usr/local/bin/go-ui
  # 撤销安装时本机放行的端口规则（ufw / firewalld / iptables）
  close_firewall_port "$PANEL_PORT" || true
  rm -rf "$PANEL_DIR"
  rm -rf "$CONF_DIR"
  if [ "${KEEP_DATA:-0}" != "1" ]; then
    rm -rf "$DATA_DIR" "$LOG_DIR"
    rm -rf "$HOME/.gost" /root/.gost 2>/dev/null || true
    rm -f /tmp/gost_install.log
  fi
  ok "卸载完成（程序、配置、go-ui 已删除；KEEP_DATA=1 可保留数据）"
}

# ---------- 主流程 ----------
main() {
  parse_args "$@"
  echo
  echo "===================== ${ORG_NAME} 安装程序 ====================="
  interactive_config
  install_gost
  install_singbox
  install_panel
  write_config
  install_systemd
  install_goui
  check_listen_port
  open_firewall_port "$PANEL_PORT"
  print_summary
}

# 按 systemd 主进程确认实际监听地址，兼容旧配置、面板内改端口及精简系统。
check_listen_port() {
  if [ "${NO_SYSTEMD:-0}" = "1" ]; then
    warn "未检测到面板监听（NO_SYSTEMD=1 时请手动启动）"
    return 0
  fi
  if ! command -v systemctl >/dev/null 2>&1; then
    warn "当前系统没有 systemd，已跳过服务监听检查"
    return 0
  fi

  local i main_pid listeners line addr detected_port code
  for i in $(seq 1 60); do
    if systemctl is-active --quiet gost-webui; then
      main_pid="$(systemctl show -p MainPID --value gost-webui 2>/dev/null || true)"
      if [ -n "$main_pid" ] && [ "$main_pid" != "0" ]; then
        if command -v ss >/dev/null 2>&1; then
          listeners="$(ss -lntp 2>/dev/null | awk -v needle="pid=$main_pid," 'index($0, needle) { print }')"
        elif command -v netstat >/dev/null 2>&1; then
          listeners="$(netstat -lntp 2>/dev/null | awk -v needle="$main_pid/" 'index($0, needle) { print }')"
        else
          ok "面板服务运行正常: systemd active（系统未提供端口探测工具）"
          return 0
        fi
        for detected_port in $(printf '%s\n' "$listeners" | awk '{print $4}' | awk -F: '{print $NF}' | tr -cd '0-9\n' | sort -u); do
          code="$(curl --noproxy '*' -sS -o /dev/null -w '%{http_code}' --max-time 1 "http://127.0.0.1:${detected_port}/" 2>/dev/null || true)"
          case "$code" in
            200|301|302|303|307|308)
              line="$(printf '%s\n' "$listeners" | awk -v suffix=":$detected_port" '$4 ~ suffix "$" { print; exit }')"
              addr="$(printf '%s\n' "$line" | awk '{print $4}')"
              PANEL_PORT="$detected_port"
              ok "面板监听正常: $addr"
              return 0
              ;;
          esac
        done
      fi
    fi
    sleep 0.5
  done

  if systemctl is-active --quiet gost-webui; then
    warn "面板服务仍在运行，但暂未检测到监听端口；最近日志如下："
  else
    warn "面板服务启动失败；最近日志如下："
  fi
  journalctl -u gost-webui -n 50 --no-pager 2>/dev/null || true
}

main "$@"
