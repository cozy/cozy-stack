#!/bin/bash
set -eu

/opt/couchdb/bin/couchdb 2> /dev/null 1> /dev/null &
MailHog 2> /dev/null 1> /dev/null &

if [ -f "/data/cozy-app/manifest.webapp" ]; then
	appdir="/data/cozy-app"
else
	show_warn=false
	for i in /data/cozy-app/*; do
		if [ ! -f "${i}/manifest.webapp" ]; then
			show_warn=true
		fi
	done
	if $show_warn; then
		appdir=""
		>&2 echo -e ""
		>&2 echo -e "WARNING:"
		>&2 echo -e "  No manifest.webapp file has been found in the mounted"
		>&2 echo -e "  directory /data/cozy-app. The stack will be started"
		>&2 echo -e "  without serving any local application."
		>&2 echo -e ""
	else
		appdir="/data/cozy-app"
	fi
fi

COZY_KONNECTORS_CMD="/usr/bin/konnector-node16-run.sh" \
COZY_ADMIN_HOST="127.0.0.1" \
COUCHDB_URL="http://admin:password@localhost:5984/" \
/usr/bin/cozy-app-dev.sh \
	-d "${appdir}" \
	-f /data/cozy-storage
