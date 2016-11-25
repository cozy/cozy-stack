#!/usr/bin/env bash

set -e

[ -z "${COZY_PROXY_HOST}" ] && COZY_PROXY_HOST="cozy.local"
[ -z "${COZY_PROXY_PORT}" ] && COZY_PROXY_PORT="8080"
[ -z "${COZY_STACK_HOST}" ] && COZY_STACK_HOST="localhost"
[ -z "${COZY_STACK_PORT}" ] && COZY_STACK_PORT="8081"

if [ -d ${COZY_STACK_PATH} ] && [ -f ${COZY_STACK_PATH}/cozy-stack ]; then
	COZY_STACK_PATH="${COZY_STACK_PATH}/cozy-stack"
fi

usage() {
	echo -e "Usage: ${0} [-h] [-d <app path>] [–v <stack version>]"

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

	check_not_running ":${COZY_PROXY_PORT}" "proxy"
	check_not_running ":${COZY_STACK_PORT}" "cozy-stack"
	do_start_couchdb
	do_create_instance
	do_start_proxy
	check_hosts

	echo "starting cozy-stack with ${vfsdir}..."

	echo ""
	echo "Go to http://app.${cozy_dev_addr}/"
	echo ""

	${COZY_STACK_PATH} serve \
		--port ${COZY_STACK_PORT} \
		--host ${COZY_STACK_HOST} \
		--couchdb-host ${COUCHDB_HOST} \
		--couchdb-port ${COUCHDB_PORT} \
		--fs-url "file://localhost${vfsdir}"
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
			kill -9 ${pid} 2>/dev/null || true
		fi
		rm "${pidfile}"
	done
}

do_start_couchdb() {
	# if COUCHDB_HOST or COUCHDB_PORT is non null, we do not try to start couchdb
	# and only check if it is accessible on the given host:port.
	if [ -n "${COUCHDB_HOST}" ] || [ -n "${COUCHDB_PORT}" ]; then
		[ -z "${COUCHDB_PORT}" ] && COUCHDB_PORT="5984"
		[ -z "${COUCHDB_HOST}" ] && COUCHDB_HOST="localhost"

		couchdb_addr="${COUCHDB_HOST}:${COUCHDB_PORT}"

		printf "checking couchdb on ${couchdb_addr}... "
		couch_test=$(curl -s -XGET "${couchdb_addr}" || echo "")
		couch_vers=$(echo "${couch_test}" | grep "\"version\":\s*\"2" || echo "")

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
		return
	fi

	COUCHDB_HOST="localhost"
	COUCHDB_PORT="5984"
	couchdb_addr="${COUCHDB_HOST}:${COUCHDB_PORT}"

	check_not_running "${couchdb_addr}" "couchdb"

	printf "starting couchdb... "

	if [ -z `command -v couchdb` ]; then
		echo "failed"
		echo_err "executable \"couchdb\" not found in \$PATH"
		exit 1
	fi

	couch_pid=`mktemp -t cozy-stack-dev.couch.XXXX` || exit 1
	trap cleanup EXIT

	couchdb 2>/dev/null 1>/dev/null &
	echo ${!} > ${couch_pid}
	wait_for "${couchdb_addr}" "couchdb"
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
	add_instance_val=$(${COZY_STACK_PATH} instances add "${cozy_dev_addr}" 2>&1)
	add_instance_ret="${?}"
	set -e
	if [ "${add_instance_ret}" = "0" ]; then
		echo "ok"
	else
		add_instance_val=$(echo "${add_instance_val}" | grep -i "already exists" || echo "")
		if [ -z "${add_instance_val}" ]; then
			echo_err "${add_instance_val} ${add_instance_ret}"
			exit 1
		fi
		echo "ok (already created)"
	fi
}

do_start_proxy() {
	site_root=`realpath ${appdir}`

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
	while ! curl -s --max-time 1 -XGET ${1} > /dev/null; do
		sleep 1
		i=$((i+1))
		if [ "${i}" -gt "10" ]; then
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
		echo ""
		echo_err "You should probaby add the following line in the /etc/hosts file:"
		echo_err "127.0.0.1\t${COZY_PROXY_HOST},app.${COZY_PROXY_HOST}"
	fi
}

echo_err() {
	>&2 echo -e "error: ${1}"
}

while getopts ":hd:f:v:" optname; do
	case "${optname}" in
	"h")
		usage
		exit 0
		;;
	"d")
		appdir="${OPTARG}"
		;;
	"f")
		vfsdir="${OPTARG}"
		;;
	"v")
		cozy_stack_version="${OPTARG}"
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

if [ -z "${vfsdir}" ]; then
	vfsdir="$(pwd)/storage"
fi

vfsdir=$(realpath "${vfsdir}")

do_start
exit 0
