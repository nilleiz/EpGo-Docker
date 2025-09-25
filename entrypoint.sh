#!/bin/sh
set -e

# --- User/Group Setup ---
# Set defaults for PUID and PGID if they are not provided
PUID=${PUID:-1001}
PGID=${PGID:-1001}

echo "Ensuring user with PUID=${PUID} and group with PGID=${PGID} exists..."

# Check if a group with the specified PGID already exists. If not, create it.
if ! getent group ${PGID} >/dev/null; then
    echo "Group with GID ${PGID} does not exist. Creating new group 'app'..."
    addgroup -g ${PGID} -S app
fi
GROUP_NAME=$(getent group ${PGID} | cut -d: -f1)

# Check if a user with the specified PUID already exists. If not, create it.
if ! getent passwd ${PUID} >/dev/null; then
    echo "User with UID ${PUID} does not exist. Creating new user 'app'..."
    adduser -u ${PUID} -G "${GROUP_NAME}" -S -h /app app
fi

# --- Initial Setup ---
chown "${PUID}:${PGID}" /app

CONFIG_FILE="/app/config.yaml"
DEFAULT_CONFIG_SRC="/usr/local/share/sample-config.yaml"

if [ ! -f "$CONFIG_FILE" ]; then
    echo "Config not found in volume. Copying default config..."
    su-exec "${PUID}:${PGID}" cp "$DEFAULT_CONFIG_SRC" "$CONFIG_FILE"
fi

# --- Execution Logic ---
EPGO_CMD="/sbin/su-exec ${PUID}:${PGID} /usr/bin/epgo -config /app/config.yaml"

# Case 1: RUN_ONCE is set to "true"
if [ "${RUN_ONCE}" = "true" ]; then
    echo "RUN_ONCE is true. Running epgo command once..."
    eval $EPGO_CMD
    echo "Task complete. Exiting."
    exit 0

# Case 2: CRON_SCHEDULE is set
elif [ -n "${CRON_SCHEDULE}" ]; then
    echo "CRON_SCHEDULE is set. Configuring cron job..."
    
    CLEAN_SCHEDULE=$(echo "${CRON_SCHEDULE}" | sed -e 's/^"//' -e 's/"$//')

    echo "${CLEAN_SCHEDULE} ${EPGO_CMD} >> /proc/1/fd/1 2>> /proc/1/fd/2" > /etc/crontabs/root
    echo "" >> /etc/crontabs/root

    NEXT_RUN_TIME=$(/usr/local/bin/nextrun "${CLEAN_SCHEDULE}")
    echo "Next execution scheduled for: ${NEXT_RUN_TIME}"
    
    echo "Starting cron daemon in the background."
    crond -b -l 8

    echo "Container is running in cron mode."
    tail -f /dev/null

# Case 3: Neither variable is set
else
    echo "Error: No execution mode defined."
    echo "Please set either the CRON_SCHEDULE or RUN_ONCE environment variable."
    exit 1
fi