#!/usr/bin/env bash
# Verify staging API DNS resolution and public health endpoints.
#
# Exits non-zero when:
#   - jarvis-api.cymonevo.com does not resolve (NXDOMAIN / empty answer)
#   - /healthz does not return HTTP 200 with "status":"ok"
#   - /readyz does not return HTTP 200 with all dependency statuses "up"
#
# Usage (from repo root):
#   scripts/verify-tunnel-dns.sh
#
# Env:
#   VERIFY_TUNNEL_HOSTNAME        (default: jarvis-api.cymonevo.com)
#   VERIFY_TUNNEL_DNS_SERVER      (default: 8.8.8.8)
#   VERIFY_TUNNEL_HEALTH_URL      (default: https://<hostname>/healthz)
#   VERIFY_TUNNEL_READY_URL       (default: https://<hostname>/readyz)
#   VERIFY_TUNNEL_DNS_CMD         override dig probe (tests)
#   VERIFY_TUNNEL_HEALTH_CMD      override health probe (tests)
#   VERIFY_TUNNEL_READY_CMD       override ready probe (tests)
#   VERIFY_TUNNEL_RETRIES         (default: 12)
#   VERIFY_TUNNEL_RETRY_DELAY     (default: 5)
set -euo pipefail

HOSTNAME="${VERIFY_TUNNEL_HOSTNAME:-jarvis-api.cymonevo.com}"
DNS_SERVER="${VERIFY_TUNNEL_DNS_SERVER:-8.8.8.8}"
HEALTH_URL="${VERIFY_TUNNEL_HEALTH_URL:-https://${HOSTNAME}/healthz}"
READY_URL="${VERIFY_TUNNEL_READY_URL:-https://${HOSTNAME}/readyz}"
RETRIES="${VERIFY_TUNNEL_RETRIES:-12}"
RETRY_DELAY="${VERIFY_TUNNEL_RETRY_DELAY:-5}"

die() {
	echo "ERROR: $*" >&2
	exit 1
}

retry() {
	local attempt=1
	local output=""
	while [ "$attempt" -le "$RETRIES" ]; do
		if output="$("$@" 2>&1)"; then
			printf '%s' "$output"
			return 0
		fi
		if [ "$attempt" -lt "$RETRIES" ]; then
			echo ">> attempt $attempt/$RETRIES failed; retrying in ${RETRY_DELAY}s ..." >&2
			sleep "$RETRY_DELAY"
		else
			printf '%s' "$output" >&2
		fi
		attempt=$((attempt + 1))
	done
	return 1
}

dns_probe() {
	if [ -n "${VERIFY_TUNNEL_DNS_CMD:-}" ]; then
		bash -c "$VERIFY_TUNNEL_DNS_CMD"
	else
		dig +short "$HOSTNAME" "@$DNS_SERVER"
	fi
}

health_probe() {
	if [ -n "${VERIFY_TUNNEL_HEALTH_CMD:-}" ]; then
		bash -c "$VERIFY_TUNNEL_HEALTH_CMD"
	else
		curl -fsS "$HEALTH_URL"
	fi
}

ready_probe() {
	if [ -n "${VERIFY_TUNNEL_READY_CMD:-}" ]; then
		bash -c "$VERIFY_TUNNEL_READY_CMD"
	else
		curl -fsS "$READY_URL"
	fi
}

verify_dns() {
	local answer
	answer="$(retry dns_probe)" || die "DNS lookup for $HOSTNAME @ $DNS_SERVER returned no records"
	if [ -z "$(printf '%s' "$answer" | tr -d '[:space:]')" ]; then
		die "DNS lookup for $HOSTNAME @ $DNS_SERVER returned empty output"
	fi
	echo ">> DNS resolves for $HOSTNAME:"
	printf '%s\n' "$answer" | sed 's/^/   /'
}

verify_health() {
	local body
	body="$(retry health_probe)" || die "health check failed for $HEALTH_URL"
	case "$body" in
	*"\"status\":\"ok\""* | *'"status":"ok"'*)
		echo ">> Health check passed for $HEALTH_URL"
		;;
	*)
		die "health check response missing status ok: $body"
		;;
	esac
}

verify_ready() {
	local body
	body="$(retry ready_probe)" || die "readiness check failed for $READY_URL"
	case "$body" in
	*"\"postgres\":\"up\""* | *'"postgres":"up"'*) ;;
	*)
		die "readiness response missing postgres up: $body"
		;;
	esac
	case "$body" in
	*"\"redis\":\"up\""* | *'"redis":"up"'*) ;;
	*)
		die "readiness response missing redis up: $body"
		;;
	esac
	echo ">> Readiness check passed for $READY_URL"
}

verify_dns
verify_health
verify_ready
echo ">> Tunnel/DNS verification passed for $HOSTNAME."
