#!/bin/bash

get_port() {
    hostname="$1"
    type="$2"

    port_var=$(echo "${hostname^^}_SERVICE_PORT_$type" | sed "s/-/_/g")
    port=${!port_var}

    if [ -z "$port" ]; then
        case $type in
            "SENTINEL")
                echo 26379
                ;;
            "REDIS")
                echo 6379
                ;;
        esac
    else
        echo $port
    fi
}

get_full_hostname() {
    hostname="$1"
    full_hostname="${hostname}.${HEADLESS_SERVICE}"
    echo "${full_hostname}"
}

REDISPORT=$(get_port "$HOSTNAME" "REDIS")

[[ -f $REDIS_PASSWORD_FILE ]] && export REDIS_PASSWORD="$(< "${REDIS_PASSWORD_FILE}")"
[[ -f $REDIS_MASTER_PASSWORD_FILE ]] && export REDIS_MASTER_PASSWORD="$(< "${REDIS_MASTER_PASSWORD_FILE}")"
if [[ -f /opt/bitnami/redis/mounted-etc/replica.conf ]];then
    cp /opt/bitnami/redis/mounted-etc/replica.conf /opt/bitnami/redis/etc/replica.conf
fi
if [[ -f /opt/bitnami/redis/mounted-etc/redis.conf ]];then
    cp /opt/bitnami/redis/mounted-etc/redis.conf /opt/bitnami/redis/etc/redis.conf
fi

echo "" >> /opt/bitnami/redis/etc/replica.conf
echo "replica-announce-port $REDISPORT" >> /opt/bitnami/redis/etc/replica.conf
echo "replica-announce-ip $(get_full_hostname "$HOSTNAME")" >> /opt/bitnami/redis/etc/replica.conf
ARGS=("--port" "${REDIS_PORT}")
ARGS+=("--replicaof" "${REDIS_MASTER_HOST}" "${REDIS_MASTER_PORT_NUMBER}")
ARGS+=("--requirepass" "${REDIS_PASSWORD}")
ARGS+=("--masterauth" "${REDIS_MASTER_PASSWORD}")
ARGS+=("--include" "/opt/bitnami/redis/etc/redis.conf")
ARGS+=("--include" "/opt/bitnami/redis/etc/replica.conf")
exec redis-server "${ARGS[@]}"
