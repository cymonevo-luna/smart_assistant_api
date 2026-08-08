#!/usr/bin/env bash
# Configure (or refresh) the Cloudflare Tunnel + DNS for the staging API hostname.
#
# Creates/updates a remotely-managed tunnel, publishes jarvis-api.cymonevo.com to
# http://127.0.0.1:8791, ensures the proxied CNAME exists, installs cloudflared
# when missing, and enables a systemd connector unit on the deploy host.
#
# The cloudflared connector MUST run on the same machine as the API compose
# stack (smart_assistant_api_compose.service) because the tunnel origin targets
# 127.0.0.1:8791 on that host.
#
# Usage (from repo root):
#   scripts/configure-cloudflare-tunnel.sh
#
# Required env (from deploy .env or the shell):
#   CLOUDFLARE_API_TOKEN
#   CLOUDFLARE_ACCOUNT_ID
#   CLOUDFLARE_ZONE_ID
#
# Optional env:
#   CLOUDFLARE_TUNNEL_NAME        (default: smart_assistant_api-staging)
#   CLOUDFLARE_TUNNEL_HOSTNAME     (default: jarvis-api.cymonevo.com)
#   CLOUDFLARE_TUNNEL_ORIGIN       (default: http://127.0.0.1:8791)
#   CLOUDFLARE_TUNNEL_SERVICE       (default: smart_assistant_api_cloudflared.service)
#   CLOUDFLARE_TUNNEL_AFTER_SERVICE (default: smart_assistant_api_compose.service)
#   CONFIGURE_TUNNEL_SKIP_INSTALL=1 skip cloudflared/systemd install (tests)
#   CONFIGURE_TUNNEL_CF_API         override Cloudflare API base URL (tests)
#   CONFIGURE_TUNNEL_CMD            override the whole configure body (tests)
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "$0")" && pwd)"
REPO_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"
ENV_FILE="${ENV_FILE:-$REPO_DIR/.env}"

TUNNEL_NAME="${CLOUDFLARE_TUNNEL_NAME:-smart_assistant_api-staging}"
TUNNEL_HOSTNAME="${CLOUDFLARE_TUNNEL_HOSTNAME:-jarvis-api.cymonevo.com}"
TUNNEL_ORIGIN="${CLOUDFLARE_TUNNEL_ORIGIN:-http://127.0.0.1:8791}"
TUNNEL_SERVICE="${CLOUDFLARE_TUNNEL_SERVICE:-smart_assistant_api_cloudflared.service}"
TUNNEL_AFTER_SERVICE="${CLOUDFLARE_TUNNEL_AFTER_SERVICE:-smart_assistant_api_compose.service}"
CF_API="${CONFIGURE_TUNNEL_CF_API:-https://api.cloudflare.com/client/v4}"

die() {
	echo "ERROR: $*" >&2
	exit 1
}

load_env_file() {
	local file="$1"
	[ -f "$file" ] || return 0

	while IFS= read -r line || [ -n "$line" ]; do
		case "$line" in
		'' | \#*) continue ;;
		esac
		if [[ ! "$line" =~ ^[A-Za-z_][A-Za-z0-9_]*= ]]; then
			continue
		fi
		local key="${line%%=*}"
		local value="${line#*=}"
		if [ -z "${!key:-}" ]; then
			export "$key=$value"
		fi
	done <"$file"
}

as_root() {
	if [ "$(id -u)" -eq 0 ]; then "$@"; else sudo "$@"; fi
}

cf_api() {
	local method="$1"
	local path="$2"
	local data="${3:-}"

	if [ -n "${CONFIGURE_TUNNEL_CF_CMD:-}" ]; then
		CONFIGURE_TUNNEL_CF_METHOD="$method" \
			CONFIGURE_TUNNEL_CF_PATH="$path" \
			CONFIGURE_TUNNEL_CF_DATA="$data" \
			bash -c "$CONFIGURE_TUNNEL_CF_CMD"
		return
	fi

	local args=(-fsS -X "$method" "${CF_API}${path}")
	args+=(-H "Authorization: Bearer ${CLOUDFLARE_API_TOKEN}")
	args+=(-H "Content-Type: application/json")
	if [ -n "$data" ]; then
		args+=(--data "$data")
	fi
	curl "${args[@]}"
}

require_cloudflare_env() {
	load_env_file "$ENV_FILE"
	for var in CLOUDFLARE_API_TOKEN CLOUDFLARE_ACCOUNT_ID CLOUDFLARE_ZONE_ID; do
		[ -n "${!var:-}" ] || die "missing required Cloudflare variable: $var"
	done
}

find_tunnel_id() {
	local response tunnel_id
	response="$(cf_api GET "/accounts/${CLOUDFLARE_ACCOUNT_ID}/cfd_tunnel?name=${TUNNEL_NAME}&is_deleted=false")"
	tunnel_id="$(printf '%s' "$response" | jq -r --arg name "$TUNNEL_NAME" '
		.result[]? | select(.name == $name) | .id' | head -n 1)"
	printf '%s' "$tunnel_id"
}

create_tunnel() {
	local response tunnel_id
	response="$(cf_api POST "/accounts/${CLOUDFLARE_ACCOUNT_ID}/cfd_tunnel" \
		"{\"name\":\"${TUNNEL_NAME}\",\"config_src\":\"cloudflare\"}")"
	if ! printf '%s' "$response" | jq -e '.success == true' >/dev/null; then
		die "failed to create tunnel: $(printf '%s' "$response" | jq -c '.errors // .')"
	fi
	tunnel_id="$(printf '%s' "$response" | jq -r '.result.id')"
	[ -n "$tunnel_id" ] && [ "$tunnel_id" != "null" ] || die "tunnel create returned no id"
	printf '%s' "$tunnel_id"
}

ensure_tunnel_id() {
	local tunnel_id
	tunnel_id="$(find_tunnel_id)"
	if [ -z "$tunnel_id" ]; then
		echo ">> Creating Cloudflare tunnel $TUNNEL_NAME ..."
		tunnel_id="$(create_tunnel)"
	else
		echo ">> Reusing Cloudflare tunnel $TUNNEL_NAME ($tunnel_id) ..."
	fi
	printf '%s' "$tunnel_id"
}

configure_tunnel_ingress() {
	local tunnel_id="$1"
	local payload response
	payload="$(jq -n \
		--arg hostname "$TUNNEL_HOSTNAME" \
		--arg origin "$TUNNEL_ORIGIN" \
		'{config:{ingress:[{hostname:$hostname,service:$origin},{service:"http_status:404"}]}}')"
	response="$(cf_api PUT "/accounts/${CLOUDFLARE_ACCOUNT_ID}/cfd_tunnel/${tunnel_id}/configurations" "$payload")"
	if ! printf '%s' "$response" | jq -e '.success == true' >/dev/null; then
		die "failed to configure tunnel ingress: $(printf '%s' "$response" | jq -c '.errors // .')"
	fi
	echo ">> Tunnel ingress configured: $TUNNEL_HOSTNAME -> $TUNNEL_ORIGIN"
}

ensure_dns_record() {
	local tunnel_id="$1"
	local target="${tunnel_id}.cfargotunnel.com"
	local response record_id existing_target
	response="$(cf_api GET "/zones/${CLOUDFLARE_ZONE_ID}/dns_records?type=CNAME&name=${TUNNEL_HOSTNAME}")"
	record_id="$(printf '%s' "$response" | jq -r '.result[0].id // empty')"
	existing_target="$(printf '%s' "$response" | jq -r '.result[0].content // empty')"

	if [ -n "$record_id" ] && [ "$existing_target" = "$target" ]; then
		echo ">> DNS record already points $TUNNEL_HOSTNAME -> $target"
		return 0
	fi

	local payload
	payload="$(jq -n \
		--arg name "$TUNNEL_HOSTNAME" \
		--arg content "$target" \
		'{type:"CNAME",name:$name,content:$content,proxied:true,ttl:1}')"

	if [ -n "$record_id" ]; then
		echo ">> Updating DNS record $TUNNEL_HOSTNAME -> $target ..."
		response="$(cf_api PUT "/zones/${CLOUDFLARE_ZONE_ID}/dns_records/${record_id}" "$payload")"
	else
		echo ">> Creating DNS record $TUNNEL_HOSTNAME -> $target ..."
		response="$(cf_api POST "/zones/${CLOUDFLARE_ZONE_ID}/dns_records" "$payload")"
	fi

	if ! printf '%s' "$response" | jq -e '.success == true' >/dev/null; then
		die "failed to configure DNS record: $(printf '%s' "$response" | jq -c '.errors // .')"
	fi
}

fetch_tunnel_token() {
	local tunnel_id="$1"
	local response token
	response="$(cf_api GET "/accounts/${CLOUDFLARE_ACCOUNT_ID}/cfd_tunnel/${tunnel_id}/token")"
	if ! printf '%s' "$response" | jq -e '.success == true' >/dev/null; then
		die "failed to fetch tunnel token: $(printf '%s' "$response" | jq -c '.errors // .')"
	fi
	token="$(printf '%s' "$response" | jq -r '.result')"
	[ -n "$token" ] && [ "$token" != "null" ] || die "tunnel token response was empty"
	printf '%s' "$token"
}

install_cloudflared() {
	if command -v cloudflared >/dev/null 2>&1; then
		return 0
	fi
	echo ">> Installing cloudflared ..."
	if command -v apt-get >/dev/null 2>&1; then
		as_root bash -c '
			set -euo pipefail
			tmp="$(mktemp)"
			curl -fsSL https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64.deb -o "$tmp"
			dpkg -i "$tmp"
			rm -f "$tmp"
		'
		return 0
	fi
	die "cloudflared is not installed and automatic install is unsupported on this host"
}

install_connector_service() {
	local token="$1"
	local unit_path="/etc/systemd/system/$TUNNEL_SERVICE"
	local cloudflared_bin
	cloudflared_bin="$(command -v cloudflared)"

	echo ">> Installing/refreshing systemd unit $unit_path ..."
	as_root tee "$unit_path" >/dev/null <<-EOF
		[Unit]
		Description=Cloudflare Tunnel for $TUNNEL_HOSTNAME
		After=network-online.target $TUNNEL_AFTER_SERVICE
		Wants=network-online.target $TUNNEL_AFTER_SERVICE

		[Service]
		Type=simple
		ExecStart=$cloudflared_bin tunnel run --token $token
		Restart=on-failure
		RestartSec=5

		[Install]
		WantedBy=multi-user.target
	EOF
	as_root systemctl daemon-reload
	as_root systemctl enable "$TUNNEL_SERVICE"
	as_root systemctl restart "$TUNNEL_SERVICE"
	as_root systemctl --no-pager --full status "$TUNNEL_SERVICE" | head -n 10 || true
}

main() {
	if [ -n "${CONFIGURE_TUNNEL_CMD:-}" ]; then
		bash -c "$CONFIGURE_TUNNEL_CMD"
		return 0
	fi

	require_cloudflare_env

	local tunnel_id token
	tunnel_id="$(ensure_tunnel_id)"
	configure_tunnel_ingress "$tunnel_id"
	ensure_dns_record "$tunnel_id"

	if [ "${CONFIGURE_TUNNEL_SKIP_INSTALL:-}" = "1" ]; then
		echo ">> Skipping cloudflared install (CONFIGURE_TUNNEL_SKIP_INSTALL=1)."
		echo ">> Cloudflare tunnel configuration completed for $TUNNEL_HOSTNAME."
		return 0
	fi

	token="$(fetch_tunnel_token "$tunnel_id")"
	install_cloudflared
	install_connector_service "$token"
	echo ">> Cloudflare tunnel configuration completed for $TUNNEL_HOSTNAME."
}

main "$@"
