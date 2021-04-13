#!/bin/bash
set -eu

DATA_DIR="/var/www/onlyoffice/Data"
LIB_DIR="/var/lib/onlyoffice"
DS_LIB_DIR="${LIB_DIR}/documentserver"

# Create app folders
for i in ${DS_LIB_DIR}/App_Data/cache/files ${DS_LIB_DIR}/App_Data/docbuilder ${DS_LIB_DIR}-example/files; do
  mkdir -p "$i"
done

# Change folder rights
for i in ${LIB_DIR} ${DATA_DIR}; do
  chown -R ds:ds "$i"
  chmod -R 755 "$i"
done

# Start services
service postgresql start
service rabbitmq-server start

# Ignore the error on restarting supervisord
documentserver-generate-allfonts.sh || true

# Export some variables for OnlyOffice
export NODE_ENV=production-linux
export NODE_CONFIG_DIR=/etc/onlyoffice/documentserver
export NODE_DISABLE_COLORS=1
export APPLICATION_NAME=ONLYOFFICE

# Start the file converter
cd /var/www/onlyoffice/documentserver/server/FileConverter
./converter &

# Start the document server
cd /var/www/onlyoffice/documentserver/server/DocService
./docservice
