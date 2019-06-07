#!/bin/sh
set -eux



echo "=========================================================================="
echo "Starting $0, $(date)"
echo "=========================================================================="



# Generate passphrase if missing
if [ ! -f /var/lib/cozy/cozy-admin-passphrase ]
then
  if [ -z "$COZY_ADMIN_PASSPHRASE" ]
  then
    echo "Using default Cozy admin passphrase !!!"
    COZY_ADMIN_PASSPHRASE="cozy"
  fi
  echo "Generating /var/lib/cozy/cozy-admin-passphrase..."
  COZY_ADMIN_PASSPHRASE="$COZY_ADMIN_PASSPHRASE" cozy-stack config passwd /var/lib/cozy/cozy-admin-passphrase
fi



# Prepare stepping down from root to applicative user with chosen UID/GID
USER_ID=${LOCAL_USER_ID:-3552}
GROUP_ID=${LOCAL_GROUP_ID:-3552}
groupadd -g $GROUP_ID -o cozy
useradd --shell /bin/bash -u $USER_ID -g cozy -o -c "Cozy Stack user" -d /var/lib/cozy -m cozy
chown -R cozy: /var/lib/cozy



# Ensuring CouchDB is ready if running an applicative subcommand
if echo "$@" | grep -q "cozy-stack "
then
  echo "Wait for CouchDB to be available..."
  wait-for-it.sh -h $COUCHDB_HOST -p $COUCHDB_PORT -t 60

  echo "Init CouchDB databases, nothing will happen if they already exists..."
  for db in _global_changes _metadata _replicator _users
  do
    curl -s -X PUT "http://${COUCHDB_USER}:${COUCHDB_PASSWORD}@${COUCHDB_HOST}:${COUCHDB_PORT}/$db"
  done

  # And also wait for Redis if defined
  if [ ! -z "$REDIS_ADDRS" ]
  then
    echo "Wait for Redis to be available..."
    wait-for-it.sh $REDIS_ADDRS -t 60
  fi

  # Then run the command itself as applicative user
  echo "Now running CMD with UID $USER_ID and GID $GROUP_ID"
  exec gosu cozy "$@"



else
  # Otherwise run the command as root
  exec "$@"
fi
