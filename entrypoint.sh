#!/bin/sh
set -e

# --- User/Group Setup ---
PUID=${PUID:-1001}
PGID=${PGID:-1001}

echo "Ensuring user with PUID=${PUID} and group with PGID=${PGID} exists..."

# Create/resolve group for PGID
if ! getent group "${PGID}" >/dev/null 2>&1; then
  echo "Group with GID ${PGID} does not exist. Creating new group 'app'..."
  addgroup -g "${PGID}" -S app
fi
GROUP_NAME="$(getent group "${PGID}" | cut -d: -f1)"

# Create/resolve user for PUID
if ! getent passwd "${PUID}" >/dev/null 2>&1; then
  echo "User with UID ${PUID} does not exist. Creating new user 'app'..."
  adduser -u "${PUID}" -G "${GROUP_NAME}" -S -h /app app
fi

# --- Initial Setup ---
chown "${PUID}:${PGID}" /app

CONFIG_FILE="/app/config.yaml"
DEFAULT_CONFIG_SRC="/usr/local/share/sample-config.yaml"

if [ ! -f "$CONFIG_FILE" ]; then
  echo "Config not found in volume. Copying default config..."
  su-exec "${PUID}:${PGID}" cp "$DEFAULT_CONFIG_SRC" "$CONFIG_FILE"
fi

# --- Helper: kill-then-run wrapper ---
# We bake the resolved PUID/PGID into the helper so cron doesn't depend on env.
HELPER="/usr/local/bin/epgo-run.sh"
cat > "${HELPER}" <<'EOF'
#!/bin/sh
set -e

# Hardcoded at generation time:
__PUID__=PUID_PLACEHOLDER
__PGID__=PGID_PLACEHOLDER

EPGO_BIN="/usr/bin/epgo"

pids_of_epgo() {
  # Prefer pidof (BusyBox-friendly); fall back to pgrep
  if command -v pidof >/dev/null 2>&1; then
    pidof epgo 2>/dev/null || true
  else
    pgrep epgo 2>/dev/null || true
  fi
}

kill_running() {
  PIDS="$(pids_of_epgo)"
  if [ -n "${PIDS}" ]; then
    echo "[epgo-run] Found running epgo (PIDs: ${PIDS}). Sending SIGTERM..."
    # SIGTERM each PID
    for p in ${PIDS}; do
      kill -TERM "$p" 2>/dev/null || true
    done
    # Wait up to 20s for graceful exit
    for i in $(seq 1 20); do
      sleep 1
      STILL=""
      for p in ${PIDS}; do
        if kill -0 "$p" 2>/dev/null; then
          STILL="yes"
          break
        fi
      done
      [ -z "$STILL" ] && break
    done
    # Force kill if any remain
    REMAIN="$(pids_of_epgo)"
    if [ -n "${REMAIN}" ]; then
      echo "[epgo-run] Still running after grace period (PIDs: ${REMAIN}). Sending SIGKILL..."
      for p in ${REMAIN}; do
        kill -KILL "$p" 2>/dev/null || true
      done
    fi
  fi
}

run_epgo() {
  echo "[epgo-run] Starting epgo..."
  exec /sbin/su-exec "${__PUID__}:${__PGID__}" sh -c 'cd /app && /usr/bin/epgo -config /app/config.yaml'
}

# Ensure we run from / to avoid cwd issues after kill
cd /
kill_running
run_epgo
EOF

# Inject numeric IDs
sed -i "s/PUID_PLACEHOLDER/${PUID}/g" "${HELPER}"
sed -i "s/PGID_PLACEHOLDER/${PGID}/g" "${HELPER}"
chmod +x "${HELPER}"

# Convenience variable used below
ECHO_NEXT_RUN="/usr/local/bin/nextrun"

# --- Execution Logic ---
if [ "${RUN_ONCE}" = "true" ]; then
  echo "RUN_ONCE is true. Executing epgo-run helper once..."
  exec "${HELPER}"

elif [ -n "${CRON_SCHEDULE}" ]; then
  echo "CRON_SCHEDULE is set. Configuring cron job..."

  CLEAN_SCHEDULE=$(echo "${CRON_SCHEDULE}" | sed -e 's/^"//' -e 's/"$//')

  # Log to container stdout/stderr, run helper directly
  echo "${CLEAN_SCHEDULE} ${HELPER} >> /proc/1/fd/1 2>> /proc/1/fd/2" > /etc/crontabs/root
  echo "" >> /etc/crontabs/root

  if [ -x "${ECHO_NEXT_RUN}" ]; then
    NEXT_RUN_TIME="$(${ECHO_NEXT_RUN} "${CLEAN_SCHEDULE}")"
    echo "Next execution scheduled for: ${NEXT_RUN_TIME}"
  fi

  echo "Starting cron daemon in the background."
  crond -b -l 8

  echo "Container is running in cron mode. Tailing to keep PID 1 alive."
  tail -f /dev/null

else
  echo "Error: No execution mode defined."
  echo "Please set either the CRON_SCHEDULE or RUN_ONCE environment variable."
  exit 1
fi
