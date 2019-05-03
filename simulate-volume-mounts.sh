#/usr/bin/env bash
COMMAND="proot"

IFS=':' read -ra MOUNTS <<< "${TELEPRESENCE_MOUNTS}"
for MOUNT in "${MOUNTS[@]}"; do
    COMMAND+=" -b ${TELEPRESENCE_ROOT}${MOUNT}:${MOUNT}"
done

COMMAND+=" bash"

echo "Mount command: $COMMAND"

exec $COMMAND
