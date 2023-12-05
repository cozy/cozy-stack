#!/bin/sh
set -eu



echo "=========================================================================="
echo "Starting $0, $(date)"
echo "=========================================================================="


# Prepare stepping down from root to applicative user with chosen UID/GID
USER_ID=${LOCAL_USER_ID:-3552}
GROUP_ID=${LOCAL_GROUP_ID:-3552}
getent group cozy >/dev/null 2>&1 || \
    groupadd -g "${GROUP_ID}" -o cozy
getent passwd cozy >/dev/null 2>&1 || \
    useradd --shell /bin/bash -u "${USER_ID}" -g cozy -o -c "Cozy Stack user" -d /var/lib/cozy -m cozy
chown -R cozy: /var/lib/cozy

# Generate passphrase if missing
if [ ! -f /etc/cozy/cozy-admin-passphrase ]; then
  if [ -z "${COZY_ADMIN_PASSPHRASE:-}" ]; then
    echo "!!!!!!!!!!!!!!!!!!!!!!!!!!!!"
    echo "COZY_ADMIN_PASSPHRASE not set."
    echo "Using random Cozy admin passphrase !!!"
    COZY_ADMIN_PASSPHRASE="$(tr -dc '[:alpha:]' </dev/urandom | fold -w 12 | head -n 1)"
    echo "COZY_ADMIN_PASSPHRASE set to ${COZY_ADMIN_PASSPHRASE}"
    echo "!!!!!!!!!!!!!!!!!!!!!!!!!!!!"
  fi
  echo "Generating /var/lib/cozy/cozy-admin-passphrase..."
  COZY_ADMIN_PASSPHRASE="${COZY_ADMIN_PASSPHRASE}" cozy-stack -c /dev/null config passwd /etc/cozy/cozy-admin-passphrase
  chown cozy: /etc/cozy/cozy-admin-passphrase
  chmod u=r,og= /etc/cozy/cozy-admin-passphrase
fi

# Generate vault keys if needed
if [ ! -f /etc/cozy/vault.enc ] || [ ! -f /etc/cozy/vault.dec ]; then
  cozy-stack -c /dev/null config gen-keys /etc/cozy/vault
  chown cozy: /etc/cozy/vault.enc /etc/cozy/vault.dec
  chmod u=rw,og= /etc/cozy/vault.enc /etc/cozy/vault.dec
fi

# Start postfix if required
if [ "${START_EMBEDDED_POSTFIX:-}" = "true" ]; then
  # Set-up dns resolution in postfix chroot at runtime and start postfix
  [ ! -d /var/spool/postfix/etc ] && mkdir -p /var/spool/postfix/etc
  cp /etc/resolv.conf /var/spool/postfix/etc
  chown -R postfix /var/spool/postfix/etc
  postfix start
fi

if echo "$@" | grep -q "cozy-stack "; then
  # Ensure CouchDB is ready if running an applicative subcommand
  echo "Waiting for CouchDB to be available..."
  wait-for-it.sh -h "${COUCHDB_HOST}" -p "${COUCHDB_PORT}" -t 60

  echo "Init CouchDB databases, nothing will happen if they already exists..."
  for db in _users _replicator; do
    curl -sSL -X PUT --user "${COUCHDB_USER}:${COUCHDB_PASSWORD}" "${COUCHDB_PROTOCOL}://${COUCHDB_HOST}:${COUCHDB_PORT}/${db}" || ( echo "Failed to create database ${db}"; exit 1 )
  done

  # Then run the command itself as applicative user
  echo "Now running CMD with UID ${USER_ID} and GID ${GROUP_ID}"
  exec gosu cozy "$@"
else
  # Otherwise run the command as root
  exec "$@"
fi
