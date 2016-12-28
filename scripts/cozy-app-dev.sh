#!/usr/bin/env bash

set -e

[ -z "${COZY_PROXY_HOST}" ] && COZY_PROXY_HOST="cozy.local"
[ -z "${COZY_PROXY_PORT}" ] && COZY_PROXY_PORT="8080"
[ -z "${COZY_STACK_HOST}" ] && COZY_STACK_HOST="localhost"
[ -z "${COZY_STACK_PORT}" ] && COZY_STACK_PORT="8081"
[ -z "${COUCHDB_PORT}" ] && COUCHDB_PORT="5984"
[ -z "${COUCHDB_HOST}" ] && COUCHDB_HOST="localhost"

if [ -d ${COZY_STACK_PATH} ] && [ -f ${COZY_STACK_PATH}/cozy-stack ]; then
	COZY_STACK_PATH="${COZY_STACK_PATH}/cozy-stack"
fi

echo_err() {
	>&2 echo -e "error: ${1}"
}

real_path() {
	[[ $1 = /* ]] && echo "$1" || echo "$PWD/${1#./}"
}

usage() {
	echo -e "Usage: ${0} [-hu] [-d <app path>] [–f <fs directory>]"
	echo -e ""
	echo -e "  -d <app path> specify the application directory to serve"
	echo -e "  -f <app path> specify the fs directory (optional)"
	echo -e "  -u try to update cozy-stack on start"
	echo -e "  -h show this usage message"
	echo -e "\nEnvironment variables"
	echo -e "\n  COZY_PROXY_HOST"
	echo -e "    specify the hostname or domain on which the proxy is listening"
	echo -e "    to incoming requests. default: cozy.local"
	echo -e "\n  COZY_PROXY_PORT"
	echo -e "    specify the port on which the proxy is listening."
	echo -e "    default: 8080."
	echo -e "\n  COZY_STACK_PATH"
	echo -e "    specify the path of the cozy-stack binary folder or the binary"
	echo -e "    itself. default: \"\$GOPATH/bin\"."
	echo -e "\n  COZY_STACK_HOST"
	echo -e "    specify the hostname on which the cozy-stack is launched."
	echo -e "    default: localhost."
	echo -e "\n  COZY_STACK_PORT"
	echo -e "    specify the port on which the cozy-stack is listening."
	echo -e "    default: 8080."
	echo -e "\n  COUCHDB_HOST"
	echo -e "    specify the host of the couchdb database. if specified,"
	echo -e "    the script won't try to start couchdb."
	echo -e "\n  COUCHDB_PORT"
	echo -e "    specify the port of the couchdb database. if specified,"
	echo -e "    the script won't try to start couchdb."
}

if [ -n "${COZY_STACK_PATH}" ] && [ ! -f "${COZY_STACK_PATH}" ]; then
	echo_err "COZY_STACK_PATH=${COZY_STACK_PATH} file does not exist"
	exit 1
fi

if [ "${COZY_STACK_PORT}" = "${COZY_PROXY_PORT}" ]; then
	echo_err "COZY_STACK_HOST and COZY_PROXY_PORT are equal"
	exit 1
fi

do_start() {
	if [ ! -f "${GOPATH}/bin/caddy" ]; then
		if [ -z `command -v go` ]; then
			echo_err "executable \"go\" not found in \$PATH"
			exit 1
		fi
		printf "installing http server (caddy)... "
		go get "github.com/mholt/caddy/caddy"
		echo "ok"
	fi

	if [ -n "${cozy_stack_version}" ]; then
		echo_err "not implemented... we do not have a release yet"
		exit 1
	fi

	if [ -z "${COZY_STACK_PATH}" ]; then
		COZY_STACK_PATH="${GOPATH}/bin/cozy-stack"
		if [ ! -f "${COZY_STACK_PATH}" ]; then
			printf "installing cozy-stack... "
			go get "github.com/cozy/cozy-stack"
			echo "ok"
		fi
	fi

	if [ "$update" = true ]; then
		printf "updating cozy-stack... "
		go get -u "github.com/cozy/cozy-stack"
		echo "ok"
	fi

	trap "trap - SIGTERM && kill -- -${$}" SIGINT SIGTERM EXIT

	check_not_running ":${COZY_PROXY_PORT}" "proxy"
	check_not_running ":${COZY_STACK_PORT}" "cozy-stack"
	do_check_couchdb
	do_start_proxy
	check_hosts

	echo "starting cozy-stack with ${vfsdir}..."

	${COZY_STACK_PATH} serve \
		--host "${COZY_STACK_HOST}" \
		--port "${COZY_STACK_PORT}" \
		--couchdb-host "${COUCHDB_HOST}" \
		--couchdb-port "${COUCHDB_PORT}" \
		--fs-url "file://localhost${vfsdir}" &

	wait_for "${COZY_STACK_HOST}:${COZY_STACK_PORT}/version/" "cozy-stack"

	do_create_instance
	echo ""
	echo "Go to http://app.${cozy_dev_addr}/"
	[ -n "${reg_token}" ] && echo "Registration token: ${reg_token}"
	cat
}

do_check_couchdb() {
	couchdb_addr="${COUCHDB_HOST}:${COUCHDB_PORT}"

	printf "waiting for couchdb..."
	wait_for "${couchdb_addr}" "couchdb"
	echo "ok"

	printf "checking couchdb on ${couchdb_addr}... "
	couch_test=$(curl -s -XGET "${couchdb_addr}" || echo "")
	couch_vers=$(grep "\"version\":\s*\"2" <<< "${couch_test}" || echo "")

	if [ -z "${couch_test}" ]; then
		echo "failed"
		echo_err "could not reach couchdb on ${couchdb_addr}"
		exit 1
	elif [ -z "${couch_vers}" ]; then
		echo "failed"
		echo_err "couchdb v1 is running on ${couchdb_addr}"
		echo_err "you need couchdb version >= 2"
		exit 1
	fi

	echo "ok"

	for dbname in "_users" "_replicator" "_global_changes"; do
		curl -s -XPUT "${couchdb_addr}/${dbname}" > /dev/null
	done
}

do_create_instance() {
	if [ "${COZY_PROXY_PORT}" = "80" ]; then
		cozy_dev_addr="${COZY_PROXY_HOST}"
	else
		cozy_dev_addr="${COZY_PROXY_HOST}:${COZY_PROXY_PORT}"
	fi

	printf "creating instance ${cozy_dev_addr}... "
	set +e
	add_instance_val=$(
		${COZY_STACK_PATH} instances add "${cozy_dev_addr}" \
			--couchdb-host "${COUCHDB_HOST}" \
			--couchdb-port "${COUCHDB_PORT}" \
			--fs-url "file://localhost${vfsdir}" 2>&1
	)
	add_instance_ret="${?}"
	set -e
	if [ "${add_instance_ret}" = "0" ]; then
		echo "ok"
		reg_token=$(grep 'token' <<< "${add_instance_val}" | sed 's/.*token: \\"\([A-Fa-f0-9]*\)\\".*/\1/g')
	else
		exists_test=$(grep -i "already exists" <<< "${add_instance_val}" || echo "")
		if [ -z "${exists_test}" ]; then
			echo_err "\n${add_instance_val} ${add_instance_ret}"
			exit 1
		fi
		echo "ok (already created)"
	fi
}

do_start_proxy() {
	site_root=$(real_path ${appdir})

	caddy_file="\n\
${COZY_PROXY_HOST} {    \n\
  proxy / ${COZY_STACK_HOST}:${COZY_STACK_PORT} {\n\
    transparent         \n\
  }                     \n\
  tls off               \n\
}                       \n\
app.${COZY_PROXY_HOST} {\n\
  root ${site_root}     \n\
  tls off               \n\
}                       \n\
"

	printf "starting caddy on \"${site_root}\"... "
	echo -e ${caddy_file} | ${GOPATH}/bin/caddy \
		-quiet \
		-conf stdin \
		-port ${COZY_PROXY_PORT} &
	wait_for "${COZY_STACK_HOST}:${COZY_PROXY_PORT}" "caddy"
	echo "ok"
}

wait_for() {
	i="0"
	while ! curl -s --max-time 0.1 -XGET ${1} > /dev/null; do
		sleep 0.1
		i=$((i+1))
		if [ "${i}" -gt "50" ]; then
			echo_err "could not listen to ${2} on ${1}"
			exit 1
		fi
	done
}

check_not_running() {
	printf "checking ${2} on ${1}... "
	if curl -s --max-time 1 -XGET ${1} > /dev/null; then
		printf "\n"
		echo_err "${2} is already running on ${1}"
		exit 1
	fi
	echo "ok"
}

check_hosts() {
	devhost=$(cat /etc/hosts | grep ${COZY_PROXY_HOST} || echo "")
	apphost=$(cat /etc/hosts | grep app.${COZY_PROXY_HOST} || echo "")
	if [ -z "${devhost}" ] || [ -z "${apphost}" ]; then
		echo -e ""
		echo -e "You should have the following line in your /etc/hosts file:"
		echo -e "127.0.0.1\t${COZY_PROXY_HOST} app.${COZY_PROXY_HOST}"
		echo -e ""
	fi
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

if [ -z "${appdir}" ]; then
	echo_err "Missing application directory argument -d"
	echo_err "Type ${0} -h"
	exit 1
fi

if [ ! -d ${appdir} ]; then
	echo_err "Application directory ${1} does not exit"
	exit 1
fi

if [ -z "${vfsdir}" ]; then
	vfsdir="$(pwd)/storage"
fi

appdir=$(real_path "${appdir}")
vfsdir=$(real_path "${vfsdir}")

do_start
exit 0
