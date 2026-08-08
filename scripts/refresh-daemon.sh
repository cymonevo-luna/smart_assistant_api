#!/usr/bin/env bash
# Generic daemon refresh script: rebuild this service's image and restart its
# systemd-managed docker compose stack. Intended to be invoked by your CI/CD
# (e.g. a Jenkins deploy job) to redeploy the service on the host.
#
# It is intentionally service-agnostic — names/paths derive from DEPLOY_APP_NAME
# (default: the repo directory name) and can each be overridden via env. Clone
# this into a service and tweak only what differs.
#
# Flow:
#   1. build <APP>:latest into the SYSTEM docker engine (the one root/systemd uses)
#   2. sync the deploy compose file + this app's .env into the deploy dir
#   3. install/refresh the systemd unit, then restart it
#
# Env overrides:
#   DEPLOY_APP_NAME     image/stack base name      (default: basename of repo dir)
#   DEPLOY_DIR          dir the daemon runs from    (default: /opt/<APP>)
#   DEPLOY_SERVICE      systemd unit name           (default: <APP>_compose.service)
#   DEPLOY_COMPOSE      compose file to deploy      (default: <repo>/docker-compose.deploy.yml,
#                                                    falling back to <repo>/docker-compose.yml)
#   DEPLOY_DOCKER_HOST  engine to build into        (default: unix:///var/run/docker.sock)
#
# The deploy webhook also exports REPOS and BRANCH for the matched push; this
# script ignores them (it always refreshes its own stack) but they are available.
#
# Usage:
#   scripts/refresh-daemon.sh             # build + deploy + restart
#   scripts/refresh-daemon.sh --no-build  # deploy + restart only
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "$0")" && pwd)"
# Repo root is the parent of scripts/ (this script lives in <repo>/scripts/).
REPO_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"

APP_NAME="${DEPLOY_APP_NAME:-$(basename "$REPO_DIR")}"
DEPLOY_DIR="${DEPLOY_DIR:-/opt/$APP_NAME}"
SERVICE="${DEPLOY_SERVICE:-${APP_NAME}_compose.service}"
SYS_DOCKER_HOST="${DEPLOY_DOCKER_HOST:-unix:///var/run/docker.sock}"

# Prefer a dedicated deploy compose (no build: sections) if present.
if [ -n "${DEPLOY_COMPOSE:-}" ]; then
	COMPOSE_FILE="$DEPLOY_COMPOSE"
elif [ -f "$REPO_DIR/docker-compose.deploy.yml" ]; then
	COMPOSE_FILE="$REPO_DIR/docker-compose.deploy.yml"
else
	COMPOSE_FILE="$REPO_DIR/docker-compose.yml"
fi
APP_ENV="$REPO_DIR/.env"

# Whether the compose stack defines a one-shot `migrate` service. When present we
# run it on every (re)start (so a completed one-shot from an older image cannot
# skip pending schema changes) and verify the resulting schema once the stack is
# up. Services without a migrate service are unaffected.
MIGRATE_SERVICE="${VERIFY_MIGRATE_SERVICE:-migrate}"
HAS_MIGRATE=0
if grep -Eq "^[[:space:]]+${MIGRATE_SERVICE}:[[:space:]]*(#.*)?$" "$COMPOSE_FILE" 2>/dev/null; then
	HAS_MIGRATE=1
fi

BUILD=1
[ "${1:-}" = "--no-build" ] && BUILD=0

# Run a command as root via sudo unless we already are root.
as_root() {
	if [ "$(id -u)" -eq 0 ]; then "$@"; else sudo "$@"; fi
}

# Run docker against the SYSTEM engine, as root (matches the systemd service).
sysdocker() {
	as_root env DOCKER_HOST="$SYS_DOCKER_HOST" docker "$@"
}

[ -f "$COMPOSE_FILE" ] || {
	echo "ERROR: compose file not found: $COMPOSE_FILE" >&2
	exit 1
}

# 1. Build the image into the system engine.
if [ "$BUILD" -eq 1 ]; then
	echo ">> Building $APP_NAME:latest into system engine ($SYS_DOCKER_HOST) ..."
	sysdocker build -t "$APP_NAME:latest" "$REPO_DIR"
else
	echo ">> Skipping image build (--no-build)."
fi

if ! sysdocker image inspect "$APP_NAME:latest" >/dev/null 2>&1; then
	echo "ERROR: $APP_NAME:latest is not present in the system engine ($SYS_DOCKER_HOST)." >&2
	echo "       Re-run without --no-build, or check your Docker engine/context." >&2
	exit 1
fi

# 2. Sync deploy files into the deploy directory.
echo ">> Syncing deploy files to $DEPLOY_DIR ..."
as_root mkdir -p "$DEPLOY_DIR"
as_root cp "$COMPOSE_FILE" "$DEPLOY_DIR/docker-compose.yml"
if [ -f "$APP_ENV" ]; then
	as_root cp "$APP_ENV" "$DEPLOY_DIR/.env"
	as_root chmod 600 "$DEPLOY_DIR/.env"
fi

VERIFY_OAUTH="$SCRIPT_DIR/verify-oauth-config.sh"
if [ -x "$VERIFY_OAUTH" ] && [ -f "$DEPLOY_DIR/.env" ]; then
	echo ">> Verifying Google OAuth configuration ..."
	ENV_FILE="$DEPLOY_DIR/.env" "$VERIFY_OAUTH"
fi

# 3. Install/refresh the systemd unit, then restart the daemon. The unit is
#    fully managed by this script, so always (re)write it to propagate changes.
#    When the stack has a migrate service, run it as ExecStartPre on every start.
MIGRATE_PRE=""
if [ "$HAS_MIGRATE" -eq 1 ]; then
	MIGRATE_PRE="ExecStartPre=/usr/bin/docker compose run --rm --no-TTY ${MIGRATE_SERVICE} up"
fi

UNIT_PATH="/etc/systemd/system/$SERVICE"
echo ">> Installing/refreshing systemd unit $UNIT_PATH ..."
as_root tee "$UNIT_PATH" >/dev/null <<-EOF
	[Unit]
	Description=$APP_NAME docker compose stack
	Requires=docker.service
	After=docker.service network-online.target
	Wants=network-online.target

	[Service]
	Type=oneshot
	RemainAfterExit=yes
	WorkingDirectory=$DEPLOY_DIR
	${MIGRATE_PRE}
	ExecStart=/usr/bin/docker compose up -d
	ExecStop=/usr/bin/docker compose down
	TimeoutStartSec=0

	[Install]
	WantedBy=multi-user.target
EOF
as_root systemctl daemon-reload
as_root systemctl enable "$SERVICE"

# With a migrate service, stop the existing stack first so a failed migrate
# one-shot cannot leave a previously started stack serving against an outdated
# schema during the restart.
if [ "$HAS_MIGRATE" -eq 1 ]; then
	echo ">> Stopping existing stack before restart ..."
	as_root env -C "$DEPLOY_DIR" docker compose down --remove-orphans 2>/dev/null || true
fi

echo ">> Restarting $SERVICE ..."
as_root systemctl restart "$SERVICE"
as_root systemctl --no-pager --full status "$SERVICE" | head -n 14 || true

# 4. Verify the deployed schema once the stack is up. Runs only when the stack
#    has a migrate service and the service ships scripts/verify-migrations.sh.
#    Service-specific gates (a required minimum version, required columns) are
#    passed via env from the calling service's refresh-daemon.sh override.
VERIFY_MIGRATIONS="$SCRIPT_DIR/verify-migrations.sh"
if [ "$HAS_MIGRATE" -eq 1 ] && [ -x "$VERIFY_MIGRATIONS" ]; then
	echo ">> Verifying database migrations ..."
	DEPLOY_DIR="$DEPLOY_DIR" DOCKER_HOST="$SYS_DOCKER_HOST" DEPLOY_APP_NAME="$APP_NAME" \
		"$VERIFY_MIGRATIONS" "${VERIFY_MIGRATIONS_MIN_VERSION:-0}"
fi

echo ">> Done."
