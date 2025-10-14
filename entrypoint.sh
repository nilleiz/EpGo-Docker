#!/bin/sh
set -e

# --- User/Group Setup ---
PUID=${PUID:-1001}
PGID=${PGID:-1001}

echo "Ensuring user with PUID=${PUID} and group with PGID=${PGID} exists..."

if ! getent group "${PGID}" >/dev/null 2>&1; then
  echo "Group with GID ${PGID} does not exist. Creating new group 'app'..."
  addgroup -g "${PGID}" -S app
fi
GROUP_NAME="$(getent group "${PGID}" | cut -d: -f1)"

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

# --- Commands ---
EPGO_CMD="cd /app && /usr/bin/epgo -config /app/config.yaml"
EPGO_EXEC="/sbin/su-exec ${PUID}:${PGID} sh -c '${EPGO_CMD}'"

# --- Cron Wrapper (kill-and-restart) ---
WRAPPER="/usr/local/bin/epgo-restart.sh"
cat > "${WRAPPER}" <<'WRAP'
#!/bin/sh
set -e

LOG(){ printf "%s epgo-cron: %s\n" "$(date -Iseconds)" "$*"; }

# Import runtime UID/GID for su-exec, written by entrypoint right before cron starts
. /run/epgo-env.sh

LOCKDIR="/var/run/epgo-cron.lock"
PIDFILE="/var/run/epgo.pid"

# prevent overlapping runs
if ! mkdir "${LOCKDIR}" 2>/dev/null; then
  LOG "previous run still active; skipping"
  exit 0
fi
trap 'rmdir "${LOCKDIR}" 2>/dev/null || true' EXIT

# Gracefully stop any running epgo owned by PUID
PIDS="$(pgrep -u "${PUID}" -f '/usr/bin/epgo' || true)"
if [ -n "${PIDS}" ]; then
  LOG "found running epgo [${PIDS}]; sending SIGTERM"
  kill -TERM ${PIDS} 2>/dev/null || true

  # wait up to 30s
  for i in $(seq 1 30); do
    sleep 1
    STILL="$(ps -o pid= -p ${PIDS} 2>/dev/null | tr -d ' \n' || true)"
    [ -z "${STILL}" ] && break
  done

  # force kill if still alive
  STILL="$(pgrep -u "${PUID}" -f '/usr/bin/epgo' || true)"
  if [ -n "${STILL}" ]; then
    LOG "epgo not exiting; sending SIGKILL"
    kill -KILL ${STILL} 2>/dev/null || true
  fi
fi

# Start fresh epgo in background
LOG "starting epgo"
# shellcheck disable=SC2086
/sbin/su-exec "${PUID}:${PGID}" sh -c 'cd /app && /usr/bin/epgo -config /app/config.yaml' \
  >> /proc/1/fd/1 2>> /proc/1/fd/2 &
NEWPID=$!
echo "${NEWPID}" > "${PIDFILE}"
LOG "epgo started with pid ${NEWPID}"
WRAP
chmod +x "${WRAPPER}"

# Export runtime values for the wrapper
mkdir -p /run
cat > /run/epgo-env.sh <<ENV
PUID=${PUID}
PGID=${PGID}
ENV

# --- Modes ---

# Case 1: RUN_ONCE
if [ "${RUN_ONCE}" = "true" ]; then
  echo "RUN_ONCE is true. Running epgo once in foreground..."
  eval "${EPGO_EXEC}"
  echo "Task complete. Exiting."
  exit 0
fi

# Case 2: CRON_SCHEDULE
if [ -n "${CRON_SCHEDULE}" ]; then
  echo "CRON_SCHEDULE is set. Configuring cron job..."

  # Strip accidental wrapping quotes
  CLEAN_SCHEDULE=$(echo "${CRON_SCHEDULE}" | sed -e 's/^"//' -e 's/"$//')

  # Write crontab: run wrapper; logs go to container stdout/stderr
  echo "${CLEAN_SCHEDULE} ${WRAPPER} >> /proc/1/fd/1 2>> /proc/1/fd/2" > /etc/crontabs/root
  echo "" >> /etc/crontabs/root

  # Try to show next run time if 'nextrun' exists
  if command -v nextrun >/dev/null 2>&1; then
    if NEXT_RUN_TIME="$(/usr/local/bin/nextrun "${CLEAN_SCHEDULE}" 2>/dev/null)"; then
      echo "Next execution scheduled for: ${NEXT_RUN_TIME}"
    else
      echo "Warning: could not compute next run time from schedule: ${CLEAN_SCHEDULE}"
    fi
  fi

  echo "Starting cron daemon in background..."
  crond -b -l 8

  # Start epgo immediately on container start;
  # cron will recycle it at the next tick.
  echo "Launching initial epgo instance now (will be restarted by cron on schedule)..."
  "${WRAPPER}"

  echo "Container is running in cron-restart mode."
  # Keep PID 1 alive
  tail -f /dev/null
fi

# Case 3: Neither variable set
echo "Error: No execution mode defined."
echo "Please set either the CRON_SCHEDULE or RUN_ONCE environment variable."
exit 1
