#!/usr/bin/env bash
# GOST 面板 卸载脚本
#
#   sudo bash uninstall.sh             # 删除程序、配置、数据、日志
#   KEEP_DATA=1 sudo bash uninstall.sh # 保留数据与日志
set -uo pipefail

PANEL_DIR="${PANEL_DIR:-/opt/gost-webui}"
CONF_DIR="${CONF_DIR:-/etc/gost-webui}"
DATA_DIR="${DATA_DIR:-/var/lib/gost-webui}"
LOG_DIR="${LOG_DIR:-/var/log/gost-webui}"

GREEN=$'\033[32m'; YELLOW=$'\033[33m'; BLUE=$'\033[34m'; RESET=$'\033[0m'
info() { echo -e "${BLUE}[*]${RESET} $*"; }
ok()   { echo -e "${GREEN}[✓]${RESET} $*"; }
warn() { echo -e "${YELLOW}[!]${RESET} $*"; }

[ "$(id -u)" = "0" ] || { echo "请使用 root 运行"; exit 1; }

info "停止并移除 systemd 服务…"
if command -v systemctl >/dev/null 2>&1; then
  systemctl stop gost-webui >/dev/null 2>&1 || true
  systemctl disable gost-webui >/dev/null 2>&1 || true
  rm -f /etc/systemd/system/gost-webui.service
  systemctl daemon-reload || true
fi

pkill -f "${PANEL_DIR}/bin/gost-webui" >/dev/null 2>&1 || true
pkill -f "${PANEL_DIR}/bin/gost" >/dev/null 2>&1 || true

info "删除程序目录 ${PANEL_DIR}"
rm -rf "$PANEL_DIR"
info "删除配置目录 ${CONF_DIR}"
rm -rf "$CONF_DIR"

if [ "${KEEP_DATA:-0}" = "1" ]; then
  warn "保留数据与日志：${DATA_DIR} ${LOG_DIR}"
else
  info "删除数据与日志 ${DATA_DIR} ${LOG_DIR}"
  rm -rf "$DATA_DIR" "$LOG_DIR"
fi

ok "卸载完成"
