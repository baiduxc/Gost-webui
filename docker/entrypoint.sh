#!/bin/sh
# Gost-webui 容器入口：修正挂载目录属主 → 首次启动生成配置 → 降权 gost 运行
set -e

# bind-mount（-v /宿主机目录:/var/lib/gost-webui）时目录常属 root，
# 直接以 gost 运行会报 mkdir/写文件 permission denied。以 root 入口统一修正属主。
if [ "$(id -u)" = "0" ]; then
  mkdir -p /var/lib/gost-webui /var/log/gost-webui /etc/gost-webui 2>/dev/null || true
  chown -R gost:gost /var/lib/gost-webui /var/log/gost-webui /etc/gost-webui 2>/dev/null || true
  RUNAS="su-exec gost"
else
  RUNAS=""
fi

CONF=/etc/gost-webui/panel.yml

rand16() { tr -dc 'A-Za-z0-9' < /dev/urandom | head -c 16; }

if [ ! -f "$CONF" ]; then
  GOST_APIK=$(rand16)
  cat > "$CONF" <<EOF
listen: ":8787"
data_dir: "/var/lib/gost-webui"
log_dir: "/var/log/gost-webui"
public_host: "${PUBLIC_HOST:-}"
sample_seconds: 15
retention_days: 90

admin:
  username: "${ADMIN_USER:-admin}"
  password: "PENDING-ROTATE"

singbox:
  bin: "/usr/local/bin/sing-box"
  dir: "/var/lib/gost-webui/singbox"

gost:
  bin: "/usr/local/bin/gost"
  config_file: "/etc/gost-webui/gost.yml"
  api_addr: "127.0.0.1:18080"
  api_username: "gost"
  api_password: "${GOS…K}"
  log_file: "/var/log/gost-webui/gost.log"
  log_level: "${GOST_LOG_LEVEL:-info}"
EOF

  # 管理员密码：优先环境变量；否则随机生成并打印到容器日志
  if [ -n "${ADMIN_PASSWORD:-}" ]; then
    ADMIN_PASS="$ADMIN_PASSWORD"
  else
    ADMIN_PASS="$(rand16)"
  fi
  $RUNAS /usr/local/bin/gost-webui -c "$CONF" -set-password "${ADMIN_USER:-admin} ${ADMIN_PASS}"
  if [ -z "${ADMIN_PASSWORD:-}" ]; then
    echo "======================================================"
    echo " [gost-webui] 初始管理员账号"
    echo "   用户名: ${ADMIN_USER:-admin}"
    echo "   密码  : ${ADMIN_PASS}"
    echo "   请登录面板后立即在「系统设置」修改"
    echo "======================================================"
  fi
  # 数据初始化需要面板本体跑一次建库；由主进程完成，这里不落库
  $RUNAS touch /var/lib/gost-webui/.first-run
fi

if [ -n "${LISTEN:-}" ]; then
  $RUNAS /usr/local/bin/gost-webui -c "$CONF" -set-listen "$LISTEN" || true
fi

exec $RUNAS /usr/local/bin/gost-webui -c "$CONF"
