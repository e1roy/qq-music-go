#!/bin/bash
# Packages QQMusic as a macOS .app with "go tool mygo build", replaces the copy
# in /Applications with it, and starts it, as 打包build.bat does on Windows.
# The bundle is ad-hoc signed, so it runs on this Mac alone; to hand it out,
# set macos.signingIdentity and macos.notarize in mygo.json (see README.md).
set -euo pipefail
cd "$(dirname "$0")"

usage() {
    cat <<'EOF'
用法：./打包build.sh [版本号] [选项]

把 QQMusicGo 打包成 macOS 的 .app，替换「/Applications/QQMusicGo.app」并启动。

  1.0.8               先把版本号写进 mygo.json（也会写进 app 的元数据）
  -u, --universal     打包 Apple 芯片与 Intel 通用的 .app（默认只打本机架构）
  -d, --dmg           另外生成分发的 dmg（默认跳过，安装用不到）
  -n, --no-install    只打包到 build/darwin-<arch>，不安装、不启动
      --no-launch     安装后不启动
  -h, --help          显示本帮助
EOF
}

UNIVERSAL=0
DMG=0
INSTALL=1
LAUNCH=1
NEWVER=""
while [ $# -gt 0 ]; do
    case "$1" in
    -h | --help)
        usage
        exit 0
        ;;
    -u | --universal) UNIVERSAL=1 ;;
    -d | --dmg) DMG=1 ;;
    -n | --no-install)
        INSTALL=0
        LAUNCH=0
        ;;
    --no-launch) LAUNCH=0 ;;
    -v | --version)
        shift
        NEWVER="${1:-}"
        ;;
    -*)
        echo "[错误] 未知选项 $1" >&2
        usage >&2
        exit 1
        ;;
    *) NEWVER="$1" ;;
    esac
    shift
done

# The shell that runs this (a login shell outside of a terminal, the editor's
# tasks) need not have go on its PATH; a Go from the official installer,
# Homebrew or an unpacked SDK is found here.
if ! command -v go >/dev/null 2>&1; then
    for candidate in /usr/local/go/bin/go /opt/homebrew/bin/go /opt/homebrew/opt/go/bin/go "$HOME"/sdk/go*/bin/go; do
        if [ -x "$candidate" ]; then
            PATH="$(dirname "$candidate"):$PATH"
            export PATH
            break
        fi
    done
fi
if ! command -v go >/dev/null 2>&1; then
    echo "[错误] 没有找到 Go，请先安装 Go: https://go.dev/dl/" >&2
    exit 1
fi

# mygo.json is read and written without jq, which macOS does not ship.
json_field() {
    perl -ne 'print "$1\n" if /"'"$1"'"\s*:\s*"([^"]*)"/' mygo.json | head -n 1
}

if [ -n "$NEWVER" ]; then
    if ! [[ $NEWVER =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        echo "[错误] 版本号格式应为 x.y.z，例如 0.2.0" >&2
        exit 1
    fi
    # \K keeps the unmatched "version": " as it is, so that the number alone is
    # the replacement: $1 beside a digit would be read as $11, $19 and so on.
    perl -0pi -e 's/"version"\s*:\s*"\K[^"]*/'"$NEWVER"'/' mygo.json
    if [ "$(json_field version)" != "$NEWVER" ]; then
        echo "[错误] 写入 mygo.json 的版本号失败" >&2
        exit 1
    fi
fi

NAME="$(json_field name)"
VERSION="$(json_field version)"
if [ -z "$NAME" ] || [ -z "$VERSION" ]; then
    echo "[错误] 无法从 mygo.json 读取名称或版本号" >&2
    exit 1
fi

if [ "$UNIVERSAL" = 1 ]; then
    PLATFORM="darwin/universal"
    OUT="build/darwin-universal"
else
    PLATFORM="darwin/$(go env GOARCH)"
    OUT="build/darwin-$(go env GOARCH)"
fi
BUNDLE="$OUT/$NAME.app"
DEST="/Applications/$NAME.app"

echo "正在打包 ${NAME} ${VERSION}（${PLATFORM}）..."
ARGS=(--platform "$PLATFORM")
if [ "$DMG" = 0 ]; then
    ARGS+=(--skip-dmg)
fi
# MyGo needs no cgo, and a build of the whole app neither.
CGO_ENABLED=0 go tool mygo build "${ARGS[@]}" || {
    echo "[错误] 打包失败" >&2
    exit 1
}

if [ ! -d "$BUNDLE" ]; then
    echo "[错误] 没有生成 $BUNDLE" >&2
    exit 1
fi

if [ "$INSTALL" = 1 ]; then
    # A running copy holds the old files and keeps the single instance lock of
    # the next launch, so it is asked to quit, then stopped.
    if pgrep -x "$NAME" >/dev/null 2>&1; then
        echo "正在关闭运行中的 $NAME ..."
        osascript -e "tell application \"$NAME\" to quit" >/dev/null 2>&1 || true
        for _ in $(seq 1 20); do
            pgrep -x "$NAME" >/dev/null 2>&1 || break
            sleep 0.25
        done
        pkill -x "$NAME" >/dev/null 2>&1 || true
        sleep 0.5
    fi

    # /Applications belongs to the admin group, so an administrator installs
    # without sudo; anything else asks for the password of an administrator.
    if [ -w /Applications ] && rm -rf "$DEST" 2>/dev/null && ditto "$BUNDLE" "$DEST" 2>/dev/null; then
        :
    else
        echo "需要管理员权限才能替换 $DEST"
        sudo -v -p "请输入管理员密码："
        sudo rm -rf "$DEST"
        sudo ditto "$BUNDLE" "$DEST"
    fi
    # A bundle that was downloaded is quarantined, and Gatekeeper would refuse
    # the ad-hoc signature that replaces the one it saw.
    xattr -dr com.apple.quarantine "$DEST" >/dev/null 2>&1 || true

    echo
    echo "已安装：${DEST}（${VERSION}）"
    if [ "$LAUNCH" = 1 ]; then
        open -a "$DEST"
        echo "已启动 $NAME"
    fi
fi

echo "打包产物：$(pwd)/$OUT"
if [ "$DMG" = 1 ]; then
    echo "   镜像  $(ls -t "$OUT"/*.dmg 2>/dev/null | head -n 1)"
fi
