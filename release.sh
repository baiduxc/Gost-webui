#!/usr/bin/env bash
# 打包发布：生成各架构预编译二进制 + 源码包（供 GitHub Release / 仓库提交使用）
#
# 用法：
#   bash release.sh                    # 默认版本号取自我方版本常量，构建 amd64/arm64
#   VERSION=1.1.1 ARCHES="amd64 arm64 arm" bash release.sh
#   bash release.sh --src-only         # 只打源码包（提交到仓库用）
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

VERSION="${VERSION:-$(grep -oE 'version = "[0-9.]+"' main.go | head -1 | grep -oE '[0-9.]+')}"
VERSION="${VERSION:-1.1.0}"
OUT="${OUT:-dist}"
ARCHES="${ARCHES:-amd64 arm64}"
PKG="gost-webui"

GREEN=$'\033[32m'; BLUE=$'\033[34m'; YELLOW=$'\033[33m'; RESET=$'\033[0m'
info() { echo -e "${BLUE}[*]${RESET} $*"; }
ok()   { echo -e "${GREEN}[✓]${RESET} $*"; }
warn() { echo -e "${YELLOW}[!]${RESET} $*"; }

command -v go >/dev/null 2>&1 || { echo "需要 Go 工具链（>= 1.26）"; exit 1; }
mkdir -p "$OUT"

pack_source() {
  info "打包源码包（用于提交仓库 / 源码安装）…"
  local files=(main.go go.mod go.sum install.sh uninstall.sh release.sh Makefile README.md DEVELOPMENT.md LICENSE THIRD_PARTY_LICENSES.md .gitignore panel.example.yml deploy internal web)
  local exist=()
  for f in "${files[@]}"; do [ -e "$f" ] && exist+=("$f"); done
  tar czf "$OUT/${PKG}-${VERSION}-src.tar.gz" "${exist[@]}"
  ok "源码包: $OUT/${PKG}-${VERSION}-src.tar.gz"
}

build_binaries() {
  for arch in $ARCHES; do
    info "编译 linux/$arch …"
    CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath \
      -ldflags="-s -w -X main.version=${VERSION}" -o "$OUT/${PKG}" .
    local name="${PKG}_${VERSION}_linux_${arch}"
    mkdir -p "$OUT/pack"
    cp "$OUT/${PKG}" "$OUT/pack/${PKG}"
    cp README.md LICENSE THIRD_PARTY_LICENSES.md "$OUT/pack/" 2>/dev/null || true
    tar czf "$OUT/${name}.tar.gz" -C "$OUT/pack" "${PKG}" README.md LICENSE THIRD_PARTY_LICENSES.md
    rm -rf "$OUT/pack"
    ok "$OUT/${name}.tar.gz"
  done
  rm -f "$OUT/${PKG}"
}

usage_notes() {
  cat <<EOF2

==================== 产物说明 ====================
$(ls -1 "$OUT" 2>/dev/null | sed 's/^/  /')

发布步骤：
  1) 提交源码到 GitHub（含 install.sh，用户即可一键安装）
       git init 2>/dev/null; git add -A && git commit -m "release v${VERSION}"
       git remote add origin https://github.com/baiduxc/${PKG}.git
       git push -u origin main

  2) （可选，推荐）上传预编译二进制到 Release，安装更快且无需在服务器上编译
       gh release create v${VERSION} ${OUT}/${PKG}_${VERSION}_linux_*.tar.gz \\
          --title "v${VERSION}" --notes "GOST 面板 v${VERSION}"

  3) 新服务器一键安装（源码构建，脚本会自动装 Go）
       curl -fsSL https://raw.githubusercontent.com/baiduxc/${PKG}/main/install.sh | sudo bash

  4) 新服务器一键安装（使用 Release 预编译包，最快）
       curl -fsSL https://raw.githubusercontent.com/baiduxc/${PKG}/main/install.sh \\
         | sudo PANEL_RELEASE_URL="https://github.com/baiduxc/${PKG}/releases/download/v${VERSION}/${PKG}_${VERSION}_linux_{arch}.tar.gz" bash

  提示：{arch} 会按机器架构自动替换。
==================================================
EOF2
}

main() {
  if [ "${1:-}" = "--src-only" ]; then
    pack_source
    usage_notes
    return
  fi
  build_binaries
  pack_source
  usage_notes
}

main "$@"
