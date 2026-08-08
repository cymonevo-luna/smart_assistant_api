#!/usr/bin/env bash
# Stop the local QA docker compose stack (volumes are preserved by default).
#
# Usage (from repo root):
#   scripts/qa-local-down.sh
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "$0")" && pwd)"
REPO_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"
QA_STATE_FILE="$REPO_DIR/.qa-local-compose"

die() {
	echo "ERROR: $*" >&2
	exit 1
}

command -v docker >/dev/null 2>&1 || die "docker is required but not installed or not on PATH"

cd "$REPO_DIR"

COMPOSE_MODE=default
if [ -f "$QA_STATE_FILE" ]; then
	COMPOSE_MODE=$(cat "$QA_STATE_FILE")
	rm -f "$QA_STATE_FILE"
fi

compose_args() {
	case "${1:-default}" in
	host)
		echo -f docker-compose.yml -f docker-compose.qa-oauth-mock.yml -f docker-compose.qa-local.yml
		;;
	*)
		echo -f docker-compose.yml -f docker-compose.qa-oauth-mock.yml
		;;
	esac
}

echo ">> Stopping docker compose stack ..."
# shellcheck disable=SC2046
if ! docker compose $(compose_args "$COMPOSE_MODE") down; then
	die "docker compose down failed"
fi

echo ">> Stack stopped."
