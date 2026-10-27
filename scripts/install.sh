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
IPV6_URL="https://github.com/${REPO}#纯-ipv6-的-vps"
# Public DNS64 servers of nat64.net, used only when the user agrees (D-136).
PUBLIC_DNS64=(2a00:1098:2b::1 2a00:1098:2c::1 2a01:4f9:c010:3f02::1)
PUBLIC_DNS64_TOS="https://nat64.net/tos"
RESOLV="/etc/resolv.conf"
RESOLV_BACKUP="/etc/resolv.conf.litenode-install"

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
# resolv_saved: RESOLV_BACKUP holds the original resolv.conf.
resolv_saved=""
cleanup() {
	if [ -n "$tmp_dir" ]; then rm -rf "$tmp_dir"; fi
	if [ -n "$tmp_bin" ]; then rm -f "$tmp_bin"; fi
	restore_resolv
}
trap cleanup EXIT
# Ctrl+C and kill still restore the DNS settings through the EXIT trap.
trap 'exit 130' INT
trap 'exit 143' TERM HUP

# restore_resolv puts back the resolv.conf moved aside by use_public_dns64
# (a file or a symlink, as it was).
restore_resolv() {
	if [ -n "$resolv_saved" ]; then
		mv -f "$RESOLV_BACKUP" "$RESOLV"
		resolv_saved=""
	fi
}

# recover_resolv puts back a resolv.conf left by a run that was killed
# before it could restore it.
recover_resolv() {
	if [ -e "$RESOLV_BACKUP" ] || [ -L "$RESOLV_BACKUP" ]; then
		mv -f "$RESOLV_BACKUP" "$RESOLV"
		yellow "已恢复上次安装中断时留下的 DNS 设置（${RESOLV}）。"
	fi
}

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

# The one-line install already needs curl; this catches running a saved
# or wget-fetched copy on a minimal system.
check_curl() {
	if ! command -v curl >/dev/null 2>&1; then
		die "未找到 curl。请先执行 apt update && apt install -y curl，再重新执行安装命令。"
	fi
}

detect_arch() {
	case "$(uname -m)" in
	x86_64 | amd64) echo amd64 ;;
	aarch64 | arm64) echo arm64 ;;
	*) die "不支持当前架构（$(uname -m)）。LiteNode 只支持 amd64 和 arm64。" ;;
	esac
}

# Extra curl options; setup_nat64 fills them.
curl_opts=()
no_ipv4=""
# public_dns64: downloading through nat64.net's DNS64 (D-136).
public_dns64=""

# nat64_prefix prints the provider's /96 NAT64 prefix, found from the
# AAAA records its DNS64 gives the IPv4-only name ipv4only.arpa (RFC 7050),
# e.g. "64:ff9b::" for 64:ff9b::c000:aa.
nat64_prefix() {
	getent ahostsv6 ipv4only.arpa 2>/dev/null | awk '
		tolower($1) ~ /^::ffff:/ { next }
		tolower($1) ~ /:c000:a[ab]$/ { sub(/c000:a[ab]$/, "", $1); print $1; exit }'
}

# setup_nat64 lets a host without an IPv4 exit download from GitHub, which
# has only IPv4 addresses, through the provider's NAT64: curl connects to
# the synthesized IPv6 addresses, TLS still checks the real host names.
# Nothing on the system is changed (DESIGN section 19.3).
setup_nat64() {
	if curl -4 -fsS --max-time 3 -o /dev/null https://1.1.1.1/cdn-cgi/trace 2>/dev/null; then
		return 0
	fi
	# A DNS64 resolver already gives github.com an IPv6 address.
	if getent ahostsv6 github.com 2>/dev/null | awk 'tolower($1) !~ /^::ffff:/ { found = 1 } END { exit !found }'; then
		return 0
	fi
	local prefix host v4
	prefix="$(nat64_prefix)"
	if [ -z "$prefix" ]; then
		no_ipv4=1
		return 0
	fi
	for host in github.com release-assets.githubusercontent.com objects.githubusercontent.com; do
		v4="$(getent ahostsv4 "$host" 2>/dev/null | awk 'NR == 1 { print $1 }')"
		if [ -n "$v4" ]; then
			curl_opts+=(--resolve "${host}:443:[${prefix}${v4}]")
		fi
	done
	echo "本机没有 IPv4 出口，经商家的 NAT64（${prefix}/96）下载。"
}

# write_public_resolv writes nat64.net's DNS servers, starting with the
# one at index first: each server answers with its own gateways.
write_public_resolv() {
	local first="$1" i n=${#PUBLIC_DNS64[@]}
	{
		echo "# Temporary DNS64 set by the LiteNode installer; restored when it ends."
		for ((i = 0; i < n; i++)); do
			echo "nameserver ${PUBLIC_DNS64[$(((first + i) % n))]}"
		done
	} >"$RESOLV"
	chmod 644 "$RESOLV"
}

# use_public_dns64 offers, when there is no IPv4 exit and no NAT64, to
# point the system DNS at nat64.net's DNS64 for the download (default no);
# cleanup puts the original resolv.conf back (D-136).
use_public_dns64() {
	if [ -z "$no_ipv4" ] || ! [ -t 0 ] || ! [ -t 1 ]; then
		return 0
	fi
	local answer
	echo "本机没有 IPv4 出口，也没有检测到商家的 NAT64，而 GitHub 只有 IPv4 地址。"
	echo "可以临时使用公共 DNS64 / NAT64 服务 nat64.net 下载：安装期间把系统 DNS 换成它的服务器，安装结束后自动换回。"
	echo "这是第三方服务，使用即表示同意它的服务条款，请先阅读：${PUBLIC_DNS64_TOS}"
	read -r -p "是否临时使用 nat64.net？[y/N]: " answer || answer=""
	case "$answer" in
	y | Y | yes | YES) ;;
	*) return 0 ;;
	esac
	mv -f "$RESOLV" "$RESOLV_BACKUP" || die "无法修改 ${RESOLV}，未做任何修改。"
	resolv_saved=1
	write_public_resolv 0
	if ! getent ahostsv6 github.com 2>/dev/null | awk 'tolower($1) !~ /^::ffff:/ { found = 1 } END { exit !found }'; then
		die "无法通过 nat64.net 的 DNS 服务器解析 github.com，已恢复原来的 DNS 设置，未做任何修改。请参考 ${IPV6_URL}"
	fi
	no_ipv4=""
	public_dns64=1
	echo "已临时改用 nat64.net 的 DNS（原设置备份在 ${RESOLV_BACKUP}）。"
}

download() {
	local name="$1" try
	if [ -n "$public_dns64" ]; then
		# nat64.net's gateways differ in load: one may reset or stall a
		# large download while another finishes in seconds. Each attempt
		# asks another DNS server first, which picks another gateway, and
		# gives up on a transfer slower than 50 KB/s for 15 seconds.
		for ((try = 0; try < ${#PUBLIC_DNS64[@]}; try++)); do
			write_public_resolv "$try"
			if curl -fsSL --connect-timeout 15 --speed-limit 51200 --speed-time 15 -o "${tmp_dir}/${name}" "${BASE_URL}/${name}"; then
				return 0
			fi
			if ((try + 1 < ${#PUBLIC_DNS64[@]})); then
				yellow "经 nat64.net 下载 ${name} 失败，换一个网关重试……"
			fi
		done
		die "经 nat64.net 下载 ${name} 失败，已恢复原来的 DNS 设置，未做任何修改。nat64.net 的网关可能繁忙，可稍后重试，或参考 ${IPV6_URL}"
	fi
	if ! curl -fsSL --retry 3 --connect-timeout 15 "${curl_opts[@]}" -o "${tmp_dir}/${name}" "${BASE_URL}/${name}"; then
		if [ -n "$no_ipv4" ]; then
			die "下载 ${name} 失败，未做任何修改。本机没有 IPv4 出口，也没有检测到商家的 NAT64，而 GitHub 只有 IPv4 地址。请参考 ${IPV6_URL}（手动上传程序后在 系统优化 → WARP 开启 IPv4 出口）"
		fi
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
	check_curl
	local arch name
	arch="$(detect_arch)"
	name="litenode-linux-${arch}"

	tmp_dir="$(mktemp -d)"
	recover_resolv
	setup_nat64
	use_public_dns64
	echo "正在下载 LiteNode（${arch}）……"
	download "$name"
	download sha256sums.txt
	verify "$name"

	mkdir -p "$BIN_DIR"
	install_binary "$name"
	install_link
	green "安装完成：$("$BIN" version)"
	echo "以后输入 litenode 或 lnode 即可打开菜单。"
	if [ -n "$public_dns64" ]; then
		restore_resolv
		echo "已恢复原来的 DNS 设置。"
		yellow "本机没有 IPv4 出口：安装 Xray、以后更新前，请先在 系统优化 → WARP 开启 IPv4 出口。"
	fi

	cleanup
	trap - EXIT
	if [ -t 0 ] && [ -t 1 ]; then
		echo
		exec "$BIN"
	fi
}

main "$@"
