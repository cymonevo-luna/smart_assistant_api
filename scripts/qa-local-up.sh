#!/usr/bin/env bash
# Bootstrap the local API stack for QA / emulator testing.
#
# Starts postgres, redis, and the API via docker compose, applies migrations,
# and waits until the health endpoint is reachable.
#
# Usage (from repo root):
#   scripts/qa-local-up.sh
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "$0")" && pwd)"
REPO_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"
ENV_FILE="$REPO_DIR/.env"
ENV_EXAMPLE="$REPO_DIR/.env.example"
QA_STATE_FILE="$REPO_DIR/.qa-local-compose"
HEALTH_URL="http://localhost:8080/healthz"
EMULATOR_BASE_URL="http://10.0.2.2:8080"
MAX_WAIT_SECONDS=60
RETRY_INTERVAL_SECONDS=2

die() {
	echo "ERROR: $*" >&2
	exit 1
}

require_command() {
	command -v "$1" >/dev/null 2>&1 || die "$1 is required but not installed or not on PATH"
}

compose_args() {
	case "${1:-default}" in
	host)
		echo -f docker-compose.yml -f docker-compose.qa-local.yml
		;;
	*)
		echo -f docker-compose.yml
		;;
	esac
}

compose_down() {
	local mode="$1"
	# shellcheck disable=SC2046
	docker compose $(compose_args "$mode") down --remove-orphans 2>/dev/null || true
}

wait_for_health() {
	local compose_mode="$1"
	echo ">> Waiting for API health check (up to ${MAX_WAIT_SECONDS}s) ..."
	local deadline=$((SECONDS + MAX_WAIT_SECONDS))
	until curl -sf "$HEALTH_URL" >/dev/null; do
		if [ "$SECONDS" -ge "$deadline" ]; then
			echo ">> Recent api logs:" >&2
			# shellcheck disable=SC2046
			docker compose $(compose_args "$compose_mode") logs --tail=50 api >&2 || true
			return 1
		fi
		sleep "$RETRY_INTERVAL_SECONDS"
	done
	echo ">> API is healthy at $HEALTH_URL"
	return 0
}

start_stack() {
	local compose_mode="$1"
	echo ">> Starting api, postgres, and redis (mode: $compose_mode) ..."
	# shellcheck disable=SC2046
	if ! docker compose $(compose_args "$compose_mode") up -d --build api postgres redis; then
		die "docker compose up failed. Check Docker logs: docker compose $(compose_args "$compose_mode") logs"
	fi
}

require_command docker
require_command curl

if ! docker info >/dev/null 2>&1; then
	die "Docker daemon is not running. Start Docker and retry."
fi

cd "$REPO_DIR"

if [ ! -f "$ENV_FILE" ]; then
	if [ ! -f "$ENV_EXAMPLE" ]; then
		die ".env is missing and .env.example was not found at $ENV_EXAMPLE"
	fi
	cp "$ENV_EXAMPLE" "$ENV_FILE"
	echo ">> Created $ENV_FILE from .env.example"
fi

COMPOSE_MODE=default
compose_down default
compose_down host

if ! start_stack default; then
	die "docker compose up failed"
fi

if ! wait_for_health default; then
	echo ">> Bridge networking health check failed; retrying with host-network API override ..." >&2
	compose_down default
	if ! start_stack host; then
		die "docker compose up failed with host-network override"
	fi
	if ! wait_for_health host; then
		die "API did not become healthy at $HEALTH_URL within ${MAX_WAIT_SECONDS}s"
	fi
	COMPOSE_MODE=host
fi

echo "$COMPOSE_MODE" >"$QA_STATE_FILE"

echo ">> Applying database migrations ..."
if ! make migrate-up; then
	die "migrations failed. Ensure postgres is reachable at localhost:5432 and retry: make migrate-up"
fi

VERIFY_OAUTH="$SCRIPT_DIR/verify-oauth-config.sh"
if [ -x "$VERIFY_OAUTH" ]; then
	get_env_var() {
		local var="$1"
		if [ -n "${!var:-}" ]; then
			printf '%s' "${!var}"
			return
		fi
		if [ -f "$ENV_FILE" ]; then
			grep -E "^${var}=" "$ENV_FILE" 2>/dev/null | tail -1 | cut -d= -f2- || true
		fi
	}

	oauth_client_id="$(get_env_var GOOGLE_OAUTH_CLIENT_ID)"
	oauth_client_secret="$(get_env_var GOOGLE_OAUTH_CLIENT_SECRET)"
	oauth_redirect_url="$(get_env_var GOOGLE_OAUTH_REDIRECT_URL)"

	if [ -n "$oauth_client_id" ] && [ -n "$oauth_client_secret" ] && [ -n "$oauth_redirect_url" ]; then
		echo ">> Verifying Google OAuth configuration ..."
		ENV_FILE="$ENV_FILE" "$VERIFY_OAUTH"
	elif [ -n "$oauth_client_id" ] || [ -n "$oauth_client_secret" ] || [ -n "$oauth_redirect_url" ]; then
		echo "WARNING: partial OAuth env detected; set GOOGLE_OAUTH_CLIENT_ID, GOOGLE_OAUTH_CLIENT_SECRET, and GOOGLE_OAUTH_REDIRECT_URL for manual OAuth QA."
	else
		echo "WARNING: OAuth env vars not set; manual OAuth QA requires GOOGLE_OAUTH_CLIENT_ID, GOOGLE_OAUTH_CLIENT_SECRET, and GOOGLE_OAUTH_REDIRECT_URL."
	fi
fi

cat <<EOF

QA local stack is ready.

  Host (curl):     http://localhost:8080
  Emulator base:   $EMULATOR_BASE_URL

Stop with: scripts/qa-local-down.sh
EOF
