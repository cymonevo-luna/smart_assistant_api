#!/usr/bin/env bash
# Verify database migration state after a deploy, as a deploy gate.
#
# Checks, in order:
#   1. the migrate tool reports a version and the schema is NOT dirty;
#   2. the version is at least a required minimum (default: 0, i.e. any version);
#   3. (optional) a set of columns exists on a table — a service-specific gate
#      wired via VERIFY_COLUMNS_TABLE + VERIFY_REQUIRED_COLUMNS.
#
# Hardened for use as a deploy gate — this runs moments after the stack is
# restarted, so probes can transiently fail while containers settle:
#   - probes are RETRIED (a one-off `docker compose run` can transiently fail to
#     create its container right after a restart);
#   - on failure the captured command output is PRINTED, never swallowed, so a
#     deploy that rolls back on this gate always logs WHY;
#   - a genuine verification failure (dirty / below-min / missing column) fails
#     immediately without retrying — only infrastructure/transient probe errors
#     are retried.
#
# This is intentionally service-agnostic: the column gate is opt-in via env, so a
# service with no such invariant just gets the version/dirty/min checks. Clone
# this into a service unchanged and pass the service-specific bits via env.
#
# Usage:
#   scripts/verify-migrations.sh [min_version]
#
# Env:
#   DEPLOY_APP_NAME         app/stack base name        (default: basename of repo dir)
#   DEPLOY_DIR              deploy dir                 (default: /opt/<APP>)
#   DOCKER_HOST             docker engine              (default: unix:///var/run/docker.sock)
#   DEPLOY_COMPOSE_FILE     compose file to probe      (default: $DEPLOY_DIR/docker-compose.yml)
#   VERIFY_MIGRATE_SERVICE  compose service that runs the migrate tool (default: migrate)
#   VERIFY_DB_SERVICE       compose service for the DB (default: postgres)
#   VERIFY_DB_NAME          database name for the columns probe (default: DEPLOY_APP_NAME)
#   VERIFY_DB_USER          db user for the columns probe        (default: postgres)
#   VERIFY_COLUMNS_TABLE    table to check columns on (optional; enables the columns gate)
#   VERIFY_REQUIRED_COLUMNS space-separated columns required on that table (optional)
#   VERIFY_MIGRATIONS_RETRIES      probe attempts before giving up (default: 10)
#   VERIFY_MIGRATIONS_RETRY_DELAY  seconds between attempts        (default: 3)
#   VERIFY_MIGRATIONS_CMD   override the "migrate version" probe (used by tests)
#   VERIFY_COLUMNS_CMD      override the columns probe           (used by tests)
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "$0")" && pwd)"
REPO_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"

MIN_VERSION="${1:-0}"
APP_NAME="${DEPLOY_APP_NAME:-$(basename "$REPO_DIR")}"
DEPLOY_DIR="${DEPLOY_DIR:-/opt/$APP_NAME}"
DOCKER_HOST="${DOCKER_HOST:-unix:///var/run/docker.sock}"
COMPOSE_FILE="${DEPLOY_COMPOSE_FILE:-$DEPLOY_DIR/docker-compose.yml}"

MIGRATE_SERVICE="${VERIFY_MIGRATE_SERVICE:-migrate}"
DB_SERVICE="${VERIFY_DB_SERVICE:-postgres}"
DB_NAME="${VERIFY_DB_NAME:-$APP_NAME}"
DB_USER="${VERIFY_DB_USER:-postgres}"

RETRIES="${VERIFY_MIGRATIONS_RETRIES:-10}"
RETRY_DELAY="${VERIFY_MIGRATIONS_RETRY_DELAY:-3}"

# Run a command as root via sudo unless we already are root.
as_root() {
	if [ "$(id -u)" -eq 0 ]; then "$@"; else sudo "$@"; fi
}

# Talk to the SYSTEM docker engine as root, exactly like refresh-daemon.sh's
# sysdocker. The deploy dir's .env is a root-owned 0600 secret and the system
# docker socket needs root, so probing as the unprivileged deploy user fails with
# "open <deploy_dir>/.env: permission denied" and docker compose exits 14 — which
# looks like a transient error but never recovers. sudo is passwordless on the
# deploy host (the rest of the deploy already relies on it).
compose() {
	as_root env DOCKER_HOST="$DOCKER_HOST" docker compose -f "$COMPOSE_FILE" "$@"
}

version_probe() {
	if [ -n "${VERIFY_MIGRATIONS_CMD:-}" ]; then
		bash -c "$VERIFY_MIGRATIONS_CMD"
	else
		compose run --rm --no-TTY "$MIGRATE_SERVICE" version
	fi
}

columns_probe() {
	if [ -n "${VERIFY_COLUMNS_CMD:-}" ]; then
		bash -c "$VERIFY_COLUMNS_CMD"
	else
		compose exec -T "$DB_SERVICE" psql -U "$DB_USER" -d "$DB_NAME" -tAc "$COLUMNS_SQL"
	fi
}

# retry_capture <label> <fn> — run fn (combining stderr into stdout) up to
# $RETRIES times, capturing its output into PROBE_OUT. Retries on ANY non-zero
# exit (the probe hitting a still-settling stack), sleeping $RETRY_DELAY between
# tries. On the final failure the raw output is printed to stderr so the caller's
# log shows the real error instead of a bare exit code. Returns the probe's exit
# status. `set +e` around the capture keeps a transient failure from tripping the
# script's own `set -e` before we can decide whether to retry.
PROBE_OUT=""
retry_capture() {
	local label="$1" fn="$2" attempt=0 rc=0
	while :; do
		attempt=$((attempt + 1))
		set +e
		PROBE_OUT="$("$fn" 2>&1)"
		rc=$?
		set -e
		[ "$rc" -eq 0 ] && return 0
		if [ "$attempt" -ge "$RETRIES" ]; then
			echo "ERROR: $label failed after $attempt attempt(s) (exit $rc):" >&2
			printf '%s\n' "$PROBE_OUT" >&2
			return "$rc"
		fi
		echo ">> $label failed (exit $rc, attempt $attempt/$RETRIES); retrying in ${RETRY_DELAY}s ..." >&2
		sleep "$RETRY_DELAY"
	done
}

if [ -z "${VERIFY_MIGRATIONS_CMD:-}" ] && [ ! -f "$COMPOSE_FILE" ]; then
	echo "ERROR: deploy compose file not found: $COMPOSE_FILE" >&2
	exit 1
fi

# 1 + 2. version present, not dirty, and at/above the required minimum.
if ! retry_capture "migrate version probe" version_probe; then
	exit 1
fi
version_output="$PROBE_OUT"

if ! printf '%s' "$version_output" | grep -Eq 'version=[0-9]+[[:space:]]+dirty=(true|false)'; then
	echo "ERROR: could not parse migrate version output:" >&2
	printf '%s\n' "$version_output" >&2
	exit 1
fi

version=$(printf '%s' "$version_output" | sed -n 's/.*version=\([0-9]*\).*/\1/p' | tail -1)
dirty=$(printf '%s' "$version_output" | sed -n 's/.*dirty=\(true\|false\).*/\1/p' | tail -1)

if [ "$dirty" = "true" ]; then
	echo "ERROR: schema_migrations is dirty at version $version; resolve before serving traffic" >&2
	exit 1
fi

if [ "$version" -lt "$MIN_VERSION" ]; then
	echo "ERROR: migration version $version is below required minimum $MIN_VERSION" >&2
	exit 1
fi

# 3. Optional columns gate — only runs when a service opts in via env.
if [ -n "${VERIFY_COLUMNS_TABLE:-}" ] && [ -n "${VERIFY_REQUIRED_COLUMNS:-}" ]; then
	read -r -a required_columns <<<"$VERIFY_REQUIRED_COLUMNS"

	# Build a quoted IN (...) list from the required columns.
	in_list=""
	for column in "${required_columns[@]}"; do
		in_list="${in_list:+$in_list, }'$column'"
	done
	COLUMNS_SQL="SELECT column_name FROM information_schema.columns
WHERE table_schema = 'public' AND table_name = '$VERIFY_COLUMNS_TABLE'
  AND column_name IN ($in_list)
ORDER BY column_name;"

	if ! retry_capture "column probe ($VERIFY_COLUMNS_TABLE)" columns_probe; then
		exit 1
	fi
	found_columns="$PROBE_OUT"

	missing=()
	for column in "${required_columns[@]}"; do
		if ! printf '%s\n' "$found_columns" | grep -qx "$column"; then
			missing+=("$column")
		fi
	done

	if [ "${#missing[@]}" -gt 0 ]; then
		echo "ERROR: table $VERIFY_COLUMNS_TABLE is missing required columns: ${missing[*]}" >&2
		echo "       apply the migration that adds them before serving traffic" >&2
		exit 1
	fi
	echo ">> Migration verification passed (version=$version dirty=$dirty, min=$MIN_VERSION, $VERIFY_COLUMNS_TABLE columns ok)."
else
	echo ">> Migration verification passed (version=$version dirty=$dirty, min=$MIN_VERSION)."
fi
