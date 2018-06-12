#!/usr/bin/env bash

set -e
set -m

[ -z "${COZY_STACK_HOST}" ] && COZY_STACK_HOST="cozy.tools"
[ -z "${COZY_STACK_PORT}" ] && COZY_STACK_PORT="8080"
[ -z "${COZY_STACK_PASS}" ] && COZY_STACK_PASS="cozy"
[ -z "${COUCHDB_URL}" ] && COUCHDB_URL="http://localhost:5984/"

if [ -d "${COZY_STACK_PATH}" ] && [ -f "${COZY_STACK_PATH}/cozy-stack" ]; then
	COZY_STACK_PATH="${COZY_STACK_PATH}/cozy-stack"
fi

echo_err() {
	>&2 echo -e "error: ${1}"
}

real_path() {
	[[ "${1}" = /* ]] && echo "${1}" || echo "${PWD}/${1#./}"
}

usage() {
	echo -e "Usage: ${0} [-hu] [-d <app path>] [–f <fs directory>]"
	echo -e ""
	echo -e "  -d <app path> specify the application directory to serve"
	echo -e "  -f <app path> specify the fs directory (optional)"
	echo -e "  -u try to update cozy-stack on start"
	echo -e "  -h show this usage message"
	echo -e "\nEnvironment variables"
	echo -e "\n  COZY_STACK_PATH"
	echo -e "    specify the path of the cozy-stack binary folder or the binary"
	echo -e "    itself. default: \"\$GOPATH/bin\"."
	echo -e "\n  COZY_STACK_HOST"
	echo -e "    specify the hostname on which the cozy-stack is launched."
	echo -e "    default: cozy.tools."
	echo -e "\n  COZY_STACK_PORT"
	echo -e "    specify the port on which the cozy-stack is listening."
	echo -e "    default: 8080."
	echo -e "\n  COZY_STACK_PASS"
	echo -e "    specify the password to register the instance with."
	echo -e "    default: cozy."
	echo -e "\n  COUCHDB_URL"
	echo -e "    specify the URL of the CouchDB database. If specified,"
	echo -e "    the script won't try to start couchdb."
}

do_start() {
	if [ -z "${COZY_STACK_PATH}" ]; then
		COZY_STACK_PATH="${GOPATH}/bin/cozy-stack"
		if [ ! -f "${COZY_STACK_PATH}" ]; then
			if [ -z "$(command -v go)" ]; then
				echo_err "executable \"go\" not found in \$PATH"
				exit 1
			fi
			printf "installing cozy-stack... "
			go get "github.com/cozy/cozy-stack"
			echo "ok"
		fi
	fi

	if [ -n "${cozy_stack_version}" ]; then
		echo_err "not implemented... we do not have a release yet"
		exit 1
	fi

	if [ "$update" = true ]; then
		printf "updating cozy-stack... "
		go get -u "github.com/cozy/cozy-stack"
		echo "ok"
	fi

	trap 'kill $(jobs -p)' SIGINT SIGTERM EXIT

	check_not_running ":${COZY_STACK_PORT}" "cozy-stack"
	do_check_couchdb

	if [ -n "${appdir}" ]; then
		if [ -f "${appdir}/manifest.webapp" ]; then
			slug="app"
		else
			appsdir=""
			for i in ${appdir}/*; do
				if [ -f "${i}/manifest.webapp" ]; then
					appsdir="${appsdir},$(basename "$i"):${i}"
				fi
				if [ -z "$slug" ]; then
					slug=$(basename "$i")
				fi
			done
			if [ -z "${appsdir}" ]; then
				echo_err "No manifest found in ${appdir}"
				exit 1
			fi
			appdir=${appsdir:1}
		fi
	fi

	echo "starting cozy-stack with ${vfsdir}..."

	${COZY_STACK_PATH} serve --allow-root \
		--appdir "${appdir}" \
		--host "${COZY_STACK_HOST}" \
		--port "${COZY_STACK_PORT}" \
		--couchdb-url "${COUCHDB_URL}" \
		--mail-host "${COZY_STACK_HOST}" \
		--mail-port 1025 \
		--mail-disable-tls \
		--fs-url "file://localhost${vfsdir}" &

	wait_for "${COZY_STACK_HOST}:${COZY_STACK_PORT}/version/" "cozy-stack"

	if [ "${COZY_STACK_PORT}" = "80" ]; then
		cozy_dev_addr="${COZY_STACK_HOST}"
	else
		cozy_dev_addr="${COZY_STACK_HOST}:${COZY_STACK_PORT}"
	fi

	echo ""
	do_create_instances
	if [ -n "${slug}" ]; then
		echo "Everything is setup. Go to http://${slug}.${cozy_dev_addr}/"
	fi
	echo "To exit, press ^C"
	fg 1 > /dev/null
}

do_check_couchdb() {
	printf "waiting for couchdb..."
	wait_for "${COUCHDB_URL}" "couchdb"
	echo "ok"

	printf "checking couchdb on %s... " "${COUCHDB_URL}"
	couch_test=$(curl -s -XGET "${COUCHDB_URL}" || echo "")
	couch_vers=$(grep "\"version\":\s*\"2" <<< "${couch_test}" || echo "")

	if [ -z "${couch_test}" ]; then
		echo "failed"
		echo_err "could not reach couchdb on ${COUCHDB_URL}"
		exit 1
	elif [ -z "${couch_vers}" ]; then
		echo "failed"
		echo_err "couchdb v1 is running on ${COUCHDB_URL}"
		echo_err "you need couchdb version >= 2"
		exit 1
	fi

	echo "ok"

	for dbname in "_users" "_replicator" "_global_changes"; do
		curl -s -XPUT "${COUCHDB_URL}/${dbname}" > /dev/null
	done
}

do_create_instances() {
	printf "creating instance %s" "${cozy_dev_addr}"
	if [ -n "${COZY_STACK_PASS}" ]; then
		printf " using passphrase \"%s\"" "${COZY_STACK_PASS}"
	fi
	printf "... "

	set +e
	add_instance_val=$(
		${COZY_STACK_PATH} instances add \
			--context-name dev \
			--dev \
			--email dev@cozy.io \
			--public-name "Jane Doe" \
			--passphrase ${COZY_STACK_PASS} \
			--domain-aliases "localhost:${COZY_STACK_PORT}" \
			"${cozy_dev_addr}" 2>&1
	)
	add_instance_ret="${?}"
	set -e
	if [ "${add_instance_ret}" = "0" ]; then
		echo "ok"
	else
		exists_test=$(grep -i "already exists" <<< "${add_instance_val}" || echo "")
		if [ -z "${exists_test}" ]; then
			echo_err "\n${add_instance_val} ${add_instance_ret}"
			exit 1
		fi
		echo "ok (already created)"
	fi
}

wait_for() {
	i="0"
	while ! LC_NUMERIC=C curl -s --max-time 0.5 -XGET "${1}" > /dev/null; do
		sleep 0.5
		i=$((i+1))
		if [ "${i}" -gt "100" ]; then
			echo_err "could not listen to ${2} on ${1}"
			exit 1
		fi
	done
}

check_not_running() {
	printf "checking that %s is free... " "${1}"
	if curl -s --max-time 1 -XGET "${1}" > /dev/null; then
		printf "\n"
		echo_err "${2} is already running on ${1}"
		exit 1
	fi
	echo "ok"
}

update=false

while getopts ":hud:f:v:" optname; do
	case "${optname}" in
	"h")
		usage
		exit 0
		;;
	"d")
		appdir="${OPTARG}"
		;;
	"u")
		update=true
		;;
	"f")
		vfsdir="${OPTARG}"
		;;
	"v")
		cozy_stack_version="${OPTARG}"
		;;
	":")
		echo_err "Option -${OPTARG} requires an argument"
		echo_err "Type ${0} -h"
		exit 1
		;;
	"?")
		echo_err "Invalid option ${OPTARG}"
		echo_err "Type ${0} -h"
		exit 1
		;;
	esac
done

if [ -n "${appdir}" ] && [ ! -d "${appdir}" ]; then
	echo_err "Application directory ${appdir} does not exit"
	exit 1
fi

if [ -z "${vfsdir}" ]; then
	vfsdir="$(pwd)/storage"
fi

[ -n "${appdir}" ] && appdir=$(real_path "${appdir}")
[ -n "${vfsdir}" ] && vfsdir=$(real_path "${vfsdir}")

do_start
exit 0
