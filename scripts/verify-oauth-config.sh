#!/usr/bin/env bash
# Verify Google OAuth environment configuration for plugin setup.
#
# Exits non-zero when required OAuth variables are missing or when the redirect
# URL uses a non-localhost host without HTTPS.
#
# Usage (from repo root):
#   scripts/verify-oauth-config.sh
#
# Env:
#   ENV_FILE              env file to load (default: <repo>/.env)
#   VERIFY_OAUTH_SKIP_HTTPS=1  skip HTTPS check (for tests)
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "$0")" && pwd)"
REPO_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"
ENV_FILE="${ENV_FILE:-$REPO_DIR/.env}"

die() {
	echo "ERROR: $*" >&2
	exit 1
}

load_env_file() {
	local file="$1"
	[ -f "$file" ] || return 0

	while IFS= read -r line || [ -n "$line" ]; do
		# Skip comments and blank lines.
		case "$line" in
		'' | \#*) continue ;;
		esac

		# Only accept KEY=VALUE assignments (no export prefix).
		if [[ ! "$line" =~ ^[A-Za-z_][A-Za-z0-9_]*= ]]; then
			continue
		fi

		local key="${line%%=*}"
		local value="${line#*=}"

		# Do not override variables already set in the environment.
		if [ -z "${!key:-}" ]; then
			export "$key=$value"
		fi
	done <"$file"
}

is_localhost_host() {
	local host="$1"
	case "$host" in
	localhost | 127.0.0.1 | [::1])
		return 0
		;;
	esac
	return 1
}

validate_redirect_url() {
	local redirect_url="$1"

	local scheme host
	scheme="$(printf '%s' "$redirect_url" | sed -n 's|^\([^:]*\)://.*|\1|p')"
	host="$(printf '%s' "$redirect_url" | sed -n 's|^[^:]*://\([^/:]*\).*|\1|p')"

	[ -n "$scheme" ] && [ -n "$host" ] || die "GOOGLE_OAUTH_REDIRECT_URL is not a valid URL: $redirect_url"

	if is_localhost_host "$host"; then
		return 0
	fi

	if [ "${VERIFY_OAUTH_SKIP_HTTPS:-}" = "1" ]; then
		return 0
	fi

	[ "$scheme" = "https" ] || die "GOOGLE_OAUTH_REDIRECT_URL must use https for non-localhost hosts (got $redirect_url)"
}

load_env_file "$ENV_FILE"

missing=()
for var in GOOGLE_OAUTH_CLIENT_ID GOOGLE_OAUTH_CLIENT_SECRET GOOGLE_OAUTH_REDIRECT_URL; do
	if [ -z "${!var:-}" ]; then
		missing+=("$var")
	fi
done

if [ "${#missing[@]}" -gt 0 ]; then
	die "missing required OAuth variables: ${missing[*]}"
fi

validate_redirect_url "$GOOGLE_OAUTH_REDIRECT_URL"

echo ">> OAuth configuration verification passed (client_id set, redirect_url=$GOOGLE_OAUTH_REDIRECT_URL)."
