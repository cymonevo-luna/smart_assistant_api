#!/usr/bin/env bash
# Seed a past-due time reminder and wait for the scheduler to dispatch it so QA
# can exercise reminder notification delivery without assistant/LLM credentials.
#
# Prerequisites:
#   - Local QA stack is running (scripts/qa-local-up.sh completed successfully)
#   - API is healthy at http://localhost:8080/healthz
#   - Database migrations 000009–000011 are applied
#
# Usage (from repo root):
#   scripts/qa-seed-reminder-notification.sh
#
# On success, prints machine-parseable QA_REMINDER_SMOKE_* variables to stdout.
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "$0")" && pwd)"
REPO_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"
QA_STATE_FILE="$REPO_DIR/.qa-local-compose"
API_BASE_URL="${QA_REMINDER_SMOKE_API_BASE_URL:-http://localhost:8080}"
HEALTH_URL="$API_BASE_URL/healthz"
POLL_INTERVAL_SECONDS=5
POLL_TIMEOUT_SECONDS=45
SMOKE_MESSAGE="QA smoke reminder notification"
PASSWORD="ReminderSmoke123!"

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
		echo -f docker-compose.yml -f docker-compose.qa-oauth-mock.yml -f docker-compose.qa-local.yml
		;;
	*)
		echo -f docker-compose.yml -f docker-compose.qa-oauth-mock.yml
		;;
	esac
}

resolve_compose_mode() {
	if [ -f "$QA_STATE_FILE" ]; then
		cat "$QA_STATE_FILE"
	else
		echo "default"
	fi
}

extract_user_id() {
	local json="$1"
	local user_id
	user_id=$(printf '%s' "$json" | jq -r '.data.id // empty')
	if [ -z "$user_id" ] || [ "$user_id" = "null" ]; then
		die "could not extract user_id from register response: $json"
	fi
	printf '%s' "$user_id"
}

extract_access_token() {
	local json="$1"
	local token
	token=$(printf '%s' "$json" | jq -r '.data.tokens.access_token // .data.access_token // empty')
	if [ -z "$token" ] || [ "$token" = "null" ]; then
		die "could not extract access_token from auth response: $json"
	fi
	printf '%s' "$token"
}

seed_past_due_reminder() {
	local user_id="$1"
	local reminder_id="$2"
	local sql
	sql=$(cat <<SQL
INSERT INTO reminders (
	id,
	user_id,
	trigger_type,
	title,
	message,
	remind_at,
	status,
	radius_meters,
	created_at,
	updated_at
) VALUES (
	'${reminder_id}'::uuid,
	'${user_id}',
	'time',
	'',
	'${SMOKE_MESSAGE}',
	now() - interval '2 minutes',
	'pending',
	0,
	now(),
	now()
);
SQL
)
	# shellcheck disable=SC2046
	if ! printf '%s\n' "$sql" | docker compose $(compose_args "$COMPOSE_MODE") exec -T postgres \
		psql -U postgres -d smart_assistant_api -v ON_ERROR_STOP=1; then
		die "failed to seed reminder in postgres (are migrations 000009-000011 applied?)"
	fi
}

wait_for_notified_reminder() {
	local deadline=$((SECONDS + POLL_TIMEOUT_SECONDS))
	local response

	echo ">> Waiting for reminder dispatch (poll every ${POLL_INTERVAL_SECONDS}s, up to ${POLL_TIMEOUT_SECONDS}s) ..."
	while [ "$SECONDS" -lt "$deadline" ]; do
		response=$(curl -sf -H "Authorization: Bearer $ACCESS_TOKEN" \
			"$API_BASE_URL/api/v1/users/me/reminders/notifications/pending")
		if printf '%s' "$response" | jq -e --arg id "$REMINDER_ID" '
			(.data // []) | map(select(.id == $id and .status == "notified")) | length > 0
		' >/dev/null; then
			echo ">> Seeded reminder is pending client delivery (status: notified)"
			return 0
		fi
		sleep "$POLL_INTERVAL_SECONDS"
	done

	die "timed out after ${POLL_TIMEOUT_SECONDS}s waiting for reminder $REMINDER_ID to appear as notified in pending notifications"
}

require_command curl
require_command jq
require_command docker
require_command uuidgen

if ! docker info >/dev/null 2>&1; then
	die "Docker daemon is not running. Start Docker and retry."
fi

cd "$REPO_DIR"
COMPOSE_MODE="$(resolve_compose_mode)"

echo ">> Checking API health at $HEALTH_URL ..."
curl -sf "$HEALTH_URL" >/dev/null || die "API is not healthy at $HEALTH_URL (run scripts/qa-local-up.sh first)"

TIMESTAMP="$(date -u +%Y%m%d%H%M%S)"
EMAIL="reminder-smoke-${TIMESTAMP}@notification.test"
REMINDER_ID="$(uuidgen | tr '[:upper:]' '[:lower:]')"

echo ">> Registering smoke user $EMAIL ..."
REGISTER_RESPONSE=$(curl -sf -X POST "$API_BASE_URL/api/v1/auth/register" \
	-H "Content-Type: application/json" \
	-d "{\"email\":\"$EMAIL\",\"name\":\"Reminder Smoke QA\",\"password\":\"$PASSWORD\"}")
USER_ID="$(extract_user_id "$REGISTER_RESPONSE")"

echo ">> Logging in smoke user ..."
LOGIN_RESPONSE=$(curl -sf -X POST "$API_BASE_URL/api/v1/auth/login" \
	-H "Content-Type: application/json" \
	-d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")
ACCESS_TOKEN="$(extract_access_token "$LOGIN_RESPONSE")"

echo ">> Seeding past-due time reminder $REMINDER_ID for user $USER_ID ..."
seed_past_due_reminder "$USER_ID" "$REMINDER_ID"

wait_for_notified_reminder

cat <<EOF

QA_REMINDER_SMOKE_EMAIL=$EMAIL
QA_REMINDER_SMOKE_PASSWORD=$PASSWORD
QA_REMINDER_SMOKE_ACCESS_TOKEN=$ACCESS_TOKEN
QA_REMINDER_SMOKE_REMINDER_ID=$REMINDER_ID
QA_REMINDER_SMOKE_API_BASE_URL=$API_BASE_URL
EOF
