#!/usr/bin/env bash
# LiteNode installer.
#
#   bash <(curl -fsSL https://raw.githubusercontent.com/Aspeternity/litenode/main/scripts/install.sh)
#
# Downloads the latest release for this architecture, verifies its sha256
# checksum, installs it as /usr/local/bin/litenode with the shortcut lnode,
# and starts the main menu. Running it again replaces the program with the
# latest release; state and configuration are not touched. Nothing is
# installed unless the download and checksum succeed.

set -euo pipefail

REPO="Aspeternity/litenode"
BASE_URL="https://github.com/${REPO}/releases/latest/download"
BIN_DIR="/usr/local/bin"
BIN="${BIN_DIR}/litenode"
LINK="${BIN_DIR}/lnode"
README_URL="https://github.com/${REPO}#无法访问-github-的机器"

# Colors only when the output (fd 1 of the call, so "red ... >&2" checks
# stderr) is a terminal and NO_COLOR is unset.
color() {
	code=$1
	shift
	if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
		printf '\033[%sm%s\033[0m\n' "$code" "$*"
	else
		printf '%s\n' "$*"
	fi
}
red() { color 31 "$@"; }
green() { color 32 "$@"; }
yellow() { color 33 "$@"; }

die() {
	red "错误：$*" >&2
	exit 1
}

tmp_dir=""
tmp_bin=""
cleanup() {
	if [ -n "$tmp_dir" ]; then rm -rf "$tmp_dir"; fi
	if [ -n "$tmp_bin" ]; then rm -f "$tmp_bin"; fi
}
trap cleanup EXIT

check_root() {
	if [ "$(id -u)" -ne 0 ]; then
		die "需要 root 权限。请切换到 root 用户（sudo -i）后重新执行安装命令。"
	fi
}

check_os() {
	local id=""
	if [ -r /etc/os-release ]; then
		# shellcheck disable=SC1091
		id="$(. /etc/os-release && echo "${ID:-}")"
	fi
	case "$id" in
	debian | ubuntu) ;;
	*) die "不支持当前系统（${id:-未知}）。LiteNode 只支持 Debian 和 Ubuntu。" ;;
	esac
}

detect_arch() {
	case "$(uname -m)" in
	x86_64 | amd64) echo amd64 ;;
	aarch64 | arm64) echo arm64 ;;
	*) die "不支持当前架构（$(uname -m)）。LiteNode 只支持 amd64 和 arm64。" ;;
	esac
}

download() {
	local name="$1"
	if ! curl -fsSL --retry 3 --connect-timeout 15 -o "${tmp_dir}/${name}" "${BASE_URL}/${name}"; then
		die "下载 ${name} 失败，未做任何修改。这台机器可能无法访问 GitHub，手动安装方法见 ${README_URL}"
	fi
}

verify() {
	local name="$1" line
	line="$(awk -v f="$name" '$2 == f || $2 == "*" f' "${tmp_dir}/sha256sums.txt")"
	if [ -z "$line" ]; then
		die "校验文件中没有 ${name} 的记录，已中止安装。"
	fi
	if ! (cd "$tmp_dir" && printf '%s\n' "$line" | sha256sum -c --quiet - >/dev/null 2>&1); then
		die "${name} 的 sha256 校验失败，文件可能已损坏或被篡改，已中止安装，未做任何修改。"
	fi
}

install_binary() {
	local name="$1"
	# Install next to the target and rename, so the replacement is atomic.
	tmp_bin="$(mktemp "${BIN_DIR}/.litenode.XXXXXX")"
	install -m 0755 "${tmp_dir}/${name}" "$tmp_bin"
	mv -f "$tmp_bin" "$BIN"
	tmp_bin=""
}

install_link() {
	if [ -L "$LINK" ]; then
		case "$(readlink "$LINK")" in
		litenode | "$BIN") return 0 ;;
		esac
	elif [ ! -e "$LINK" ]; then
		ln -s litenode "$LINK"
		return 0
	fi
	yellow "提示：${LINK} 已被其他程序占用，未创建快捷命令 lnode，请使用 litenode 命令。"
}

main() {
	check_root
	check_os
	local arch name
	arch="$(detect_arch)"
	name="litenode-linux-${arch}"

	tmp_dir="$(mktemp -d)"
	echo "正在下载 LiteNode（${arch}）……"
	download "$name"
	download sha256sums.txt
	verify "$name"

	mkdir -p "$BIN_DIR"
	install_binary "$name"
	install_link
	green "安装完成：$("$BIN" version)"
	echo "以后输入 litenode 或 lnode 即可打开菜单。"

	cleanup
	trap - EXIT
	if [ -t 0 ] && [ -t 1 ]; then
		echo
		exec "$BIN"
	fi
}

main "$@"
