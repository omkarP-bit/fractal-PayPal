#!/bin/sh
# Container entrypoint for the FRACTAL API.
#
# 1. Apply database migrations (idempotent; safe on every boot).
# 2. Start the embedded OPA server unless an external OPA_URL was supplied.
# 3. Exec the API.
set -eu

log() { echo "[entrypoint] $*"; }

# Render injects PORT and expects the service to bind to it. Honour it here so
# the same image works on Render, docker compose and plain local runs.
if [ -z "${FRACTAL_PORT:-}" ]; then
	FRACTAL_PORT="${PORT:-8080}"
	export FRACTAL_PORT
fi

if [ -z "${DATABASE_URL:-}" ]; then
	log "DATABASE_URL is not set; skipping migrations"
else
	log "applying database migrations"
	/bin/migrate || {
		status=$?
		log "migrations failed (exit $status); refusing to start the API"
		exit "$status"
	}
fi

# Local compose points OPA_URL at the dedicated opa service. Render has no
# such service, so run the embedded binary and keep the same HTTP contract.
if [ -z "${OPA_URL:-}" ]; then
	OPA_URL="http://127.0.0.1:8181"
	export OPA_URL
fi

EMBEDDED_OPA="${OPA_EMBEDDED:-true}"
if [ "$EMBEDDED_OPA" = "true" ] && [ "$OPA_URL" = "http://127.0.0.1:8181" ]; then
	log "starting embedded OPA on 127.0.0.1:8181"
	/opa run --server --addr=127.0.0.1:8181 --log-level=error /policies &
	OPA_PID=$!

	# Wait briefly for the policy engine so the first request does not fail.
	ready=false
	i=0
	while [ "$i" -lt 50 ]; do
		if wget -q -O /dev/null "http://127.0.0.1:8181/health" 2>/dev/null; then
			ready=true
			break
		fi
		if ! kill -0 "$OPA_PID" 2>/dev/null; then
			log "embedded OPA exited before becoming healthy"
			break
		fi
		i=$((i + 1))
		sleep 0.2
	done
	if [ "$ready" = true ]; then
		log "embedded OPA is ready"
	else
		log "embedded OPA did not report healthy; the API will start anyway"
	fi
elif [ "$EMBEDDED_OPA" = "true" ]; then
	log "using external OPA at $OPA_URL"
fi

exec /bin/fractal-api
