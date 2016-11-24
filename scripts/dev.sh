#!/usr/bin/env bash

[ -z ${COZY_DEV_HOST} ] && COZY_DEV_HOST="cozy.local"
[ -z ${COZY_DEV_PORT} ] && COZY_DEV_PORT="8080"
[ -z ${COZY_STACK_HOST} ] && COZY_STACK_HOST="localhost"
[ -z ${COZY_STACK_PORT} ] && COZY_STACK_PORT="8081"

[ -z ${COUCHDB_HOST} ] && COUCHDB_HOST="localhost"
[ -z ${COUCHDB_PORT} ] && COUCHDB_PORT="5984"
[ -z ${COUCHDB_ENABLE} ] && COUCHDB_ENABLE="1"

if [ -d ${COZY_STACK_PATH} ] && [ -f ${COZY_STACK_PATH}/cozy-stack ]; then
	COZY_STACK_PATH="${COZY_STACK_PATH}/cozy-stack"
fi

usage() {
	echo -e "Usage: ${0} [-h] [-d <app path>] [–v <stack version>]"

	echo -e "\nEnvironment variables"
	echo -e "\n  COZY_DEV_HOST"
	echo -e "    specify the hostname or domain on which the dev server is listening"
	echo -e "    to incoming requests. default: cozy.local"
	echo -e "\n  COZY_DEV_PORT"
	echo -e "    specify the port on which the dev server is listening."
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
	echo -e "\n  COUCHDB_ENABLE"
	echo -e "    specify whether or not this script should launch couchdb."
	echo -e "    default: 1"
	echo -e "\n  COUCHDB_HOST"
	echo -e "    specify the host of the couchdb database. default: localhost"
	echo -e "\n  COUCHDB_PORT"
	echo -e "    specify the port of the couchdb database. default: 5984"
}

if [ -n "${COZY_STACK_PATH}" ] && [ ! -f "${COZY_STACK_PATH}" ]; then
	echo_err "COZY_STACK_PATH=${COZY_STACK_PATH} file does not exist"
	exit 1
fi

if [ "${COZY_STACK_PORT}" = "${COZY_DEV_PORT}" ]; then
	echo_err "COZY_STACK_HOST and COZY_DEV_PORT are equal"
	exit 1
fi

do_start() {
	set -e

	if [ ! -f "${GOPATH}/bin/caddy" ]; then
		if [ -z `command -v go` ]; then
			echo_err "Executable \"go\" not found in \$PATH"
			exit 1
		fi
		printf "installing http server (caddy)... "
		go get "github.com/mholt/caddy/caddy"
		echo "ok"
	fi

	if [ -n "${cozy_stack_version}" ]; then
		echo_err "Not implemented... we do not have a release yet"
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

	do_start_couchdb
	do_start_proxy
	check_hosts

	echo ""
	echo "Go to http://app.${COZY_DEV_HOST}:${COZY_DEV_PORT}/"
	echo ""

	${COZY_STACK_PATH} serve \
		--port ${COZY_STACK_PORT} \
		--host ${COZY_STACK_HOST} \
		--couchdb-host ${COUCHDB_HOST} \
		--couchdb-port ${COUCHDB_PORT}
}

cleanup() {
	for tmpdir in "${TMPDIR}" "${TMP}" /var/tmp /tmp; do
		test -d "${tmpdir}" && break
	done

	pids=`find ${tmpdir} -iname cozy-stack-dev.couch*`

	for pidfile in ${pids}; do
		pid=`cat "${pidfile}"`
		if [ -n "${pid}" ]; then
			echo "stopping couchdb"
			kill -9 ${pid} 2>/dev/null 1>/dev/null || true
		fi
		rm "${pidfile}"
	done
}

do_start_couchdb() {
	if [ "${COUCHDB_ENABLE}" = "0" ]; then
		echo "skip couchdb"
		return
	fi

	printf "checking couchdb... "

	set +e
	couch_test=`curl -s -XGET "${COUCHDB_HOST}:${COUCHDB_PORT}"`
	if [ -n "${couch_test}" ]; then
		couch_test=`echo "${couch_test}" | grep "\"version\":\s*\"2"`
		if [ -z "${couch_test}" ]; then
			echo_err ""
			echo_err "couchdb v1 is running on ${COUCHDB_HOST}:${COUCHDB_PORT}"
			echo_err "you need couchdb version >= 2"
			exit 1
		else
			echo "ok"
			return
		fi
	fi
	set -e

	if [ -z `command -v go` ]; then
		echo_err "\nExecutable \"couchdb\" not found in \$PATH"
		exit 1
	fi

	couch_pid=`mktemp -t cozy-stack-dev.couch.XXXX` || exit 1
	trap cleanup EXIT

	printf "none found, starting... "
	couchdb 2> /dev/null 1> /dev/null &
	echo ${!} > ${couch_pid}
	wait_for "${COUCHDB_HOST}:${COUCHDB_PORT}" "couchdb"
	echo "ok"

	for i in "_users" "_replicator" "_global_changes"; do
		curl -s -XPUT "${COUCHDB_HOST}:${COUCHDB_PORT}/${i}" > /dev/null
	done
}

do_start_proxy() {
	site_root=`realpath ${appdir}`

	check_not_running "${COZY_DEV_HOST}:${COZY_DEV_PORT}" "dev server"
	check_not_running "${COZY_STACK_HOST}:${COZY_STACK_PORT}" "cozy-stack"

	caddy_file="\n\
${COZY_DEV_HOST} {      \n\
  proxy / ${COZY_STACK_HOST}:${COZY_STACK_PORT} \n\
  tls off               \n\
}                       \n\
app.${COZY_DEV_HOST} {  \n\
  root ${site_root}     \n\
  tls off               \n\
}                       \n\
 "

	printf "starting caddy on \"${site_root}\"... "
	echo ${caddy_file} | ${GOPATH}/bin/caddy \
		-quiet \
		-conf stdin \
		-port ${COZY_DEV_PORT} &
	wait_for "${COZY_STACK_HOST}:${COZY_DEV_PORT}" "caddy"
	echo "ok"
}

wait_for() {
	i="0"
	while ! curl -s --max-time 1 -XGET ${1} > /dev/null; do
		sleep 0.5
		i=$((i+1))
		if [ "${i}" -gt "10" ]; then
			echo_err "could not listen to ${2} on ${1}"
			exit 1
		fi
	done
}

check_not_running() {
	printf "checking ${1}... "
	if curl -s --max-time 1 -XGET ${1} > /dev/null; then
		printf "\n"
		echo_err "${2} is already running on ${1}"
		exit 1
	fi
	echo "ok"
}

check_hosts() {
	set +e
	devhost=`cat /etc/hosts | grep ${COZY_DEV_HOST}`
	apphost=`cat /etc/hosts | grep app.${COZY_DEV_HOST}`
	if [ -z "${devhost}" ] || [ -z "${apphost}" ]; then
		echo ""
		echo_err "You should probaby add the following line in the /etc/hosts file:"
		echo_err "127.0.0.1\t${COZY_DEV_HOST},app.${COZY_DEV_HOST}"
	fi
	set -e
}

echo_err() {
	>&2 echo -e "error: ${1}"
}

while getopts ":hd:v:" optname; do
	case "${optname}" in
	"h")
		usage
		exit 0
		;;
	"d")
		appdir=${OPTARG}
		;;
	"v")
		cozy_stack_version=${OPTARG}
		;;
	":")
		echo_err "Option -${OPTARG} requires an argument"
		echo_err "Type ${0} --help"
		exit 1
		;;
	"?")
		echo_err "Invalid option ${OPTARG}"
		echo_err "Type ${0} --help"
		exit 1
		;;
	esac
done

if [ -z "${appdir}" ]; then
	echo_err "Missing application directory argument -d"
	echo_err "Type ${0} --help"
	exit 1
fi

if [ ! -d ${appdir} ]; then
	echo_err "Application directory ${1} does not exit"
	exit 1
fi

do_start
exit 0
