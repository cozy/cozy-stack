#!/usr/bin/env bash
set -e

COZY_ENV_DFL=production

[ -z "${COZY_ENV}" ] && COZY_ENV="${COZY_ENV_DFL}"
[ -z "${COZY_DEPLOY_USER}" ] && COZY_DEPLOY_USER="${USER}"

pushd `dirname $0` > /dev/null
WORK_DIR=$(dirname "`pwd`")
popd > /dev/null
ASSETS=./assets

if [ -r "${WORK_DIR}/local.env" ]; then
	. "${WORK_DIR}/local.env"
fi

usage() {
	echo -e "Usage: ${1} [release] [deploy] [clean]"
	echo -e "\nCommands:\n"
	echo -e "  release  builds a release of the current working-tree"
	echo -e "  deploy   builds a release of the current working-tree and deploys it"
	echo -e "  clean    remove all generated files from the working-tree"

	echo -e "\nEnvironment variables:"
	echo -e "\n  COZY_ENV"
	echo -e "    with release command, specify the environment of the release."
	echo -e "    can be \"production\" or \"development\". default: \"${COZY_ENV_DFL}\""
	echo -e "\n  COZY_DEPLOY_USER"
	echo -e "    with deploy command, specify the user used to deploy."
	echo -e "    default: \$USER (${USER})"
	echo -e "\n  COZY_DEPLOY_SERVER"
	echo -e "    with deploy command, specify the ssh server to deploy on."
	echo -e "\n  COZY_DEPLOY_PROXY"
	echo -e "    with deploy command, specify an ssh proxy to go through."
	echo -e "\n  COZY_DEPLOY_POSTSCRIPT"
	echo -e "    with deploy command, specify an optional script to execute"
	echo -e "    on the deploy server after deploying."
}

# The version string is deterministic and reflects entirely the state
# of the working-directory from which the release is built from. It is
# generated using the following format:
#
# 		<TAG>[-<NUMBER OF COMMITS AFTER TAG>][-dirty][-dev]
#
# Where:
#  - <TAG>: closest annotated tag of the current working directory. If
#    no tag is present, is uses the string "v0". This is not allowed
#    in a production release.
#  - <NUMBER OF COMMITS AFTER TAG>: number of commits after the
#    closest tag if the current working directory does not point
#    exactly to a tag
#  - -dirty: added if the working if the working-directory is not
#    clean (contains un-commited modifications). This is not allowed
#    in production release.
#  - -dev: added for a development mode relase
#
# The outputed binary is named "cozy-stack-${VERSION_STRING}". A
# SHA256 checksum of the binary is also generated in a file named
# "cozy-stack-${VERSION_STRING}.sha256".
do_release() {
	check_env

	VERSION_STRING=`git --git-dir="${WORK_DIR}/.git" --work-tree="${WORK_DIR}" \
		describe --tags --dirty 2> /dev/null | \
		sed -E 's/(.*)-g[[:xdigit:]]+(-?.*)$/\1\2/g'`

	if [ "${VERSION_STRING}" == "" ]; then
		if [ "${COZY_ENV}" == production ]; then
			>&2 echo "ERR: Can not build a production release without a tagged version"
			exit 1
		fi
		VERSION_STRING=v0-`git rev-parse --short HEAD`
		>&2 echo "WRN: No tag has been found to version the stack, using \"${VERSION_STRING}\" as version number"
	fi

	if [ `git diff --shortstat HEAD 2> /dev/null | tail -n1 | wc -l` -gt 0 ]; then
		if [ "${COZY_ENV}" == production ]; then
			>&2 echo "ERR: Can not build a production release in a dirty work-tree"
			exit 1
		fi
		VERSION_STRING="${VERSION_STRING}-dirty"
	fi

	if [ "${COZY_ENV}" == development ]; then
		VERSION_STRING="${VERSION_STRING}-dev"
	fi

	BINARY="cozy-stack-${VERSION_STRING}"
	BUILD_TIME=`date -u +"%Y-%m-%dT%H:%M:%SZ"`
	BUILD_MODE="${COZY_ENV}"

	rm ./web/statik/statik.go
	go generate ./web/routing

	go build -ldflags "\
		-X github.com/cozy/cozy-stack/pkg/config.Version=${VERSION_STRING} \
		-X github.com/cozy/cozy-stack/pkg/config.BuildTime=${BUILD_TIME} \
		-X github.com/cozy/cozy-stack/pkg/config.BuildMode=${BUILD_MODE}
		" \
		-o "${BINARY}"

	openssl dgst -sha256 -hex "${BINARY}" > "${BINARY}.sha256"

	printf "${BINARY}\t"
	cat "${BINARY}.sha256" | sed -E 's/SHA256\((.*)\)= ([a-f0-9]+)$/\2/g'
}

# The deploy command will build a new release and deploy it on a
# distant server using scp. To configure the distance server, you can
# use the environment variables (see help usage):
#
#  - COZY_DEPLOY_USER: deploy user (default to $USER)
#  - COZY_DEPLOY_SERVER: deploy server
#  - COZY_DEPLOY_PROXY: deploy proxy (optional)
#  - COZY_DEPLOY_POSTSCRIPT: deploy script to execute after deploy
#    (optional)
#
do_deploy() {
	check_env

	do_release

	if [ -z "${COZY_DEPLOY_PROXY}" ]; then
		scp "${BINARY}" "${COZY_DEPLOY_USER}@${COZY_DEPLOY_SERVER}:cozy-stack"
	else
		scp -oProxyCommand="ssh -W %h:%p ${COZY_DEPLOY_PROXY}" "${BINARY}" "${COZY_DEPLOY_USER}@${COZY_DEPLOY_SERVER}:cozy-stack"
	fi

	if [ -n "${COZY_DEPLOY_POSTSCRIPT}" ]; then
		if [ -z "${COZY_DEPLOY_PROXY}" ]; then
			ssh "${COZY_DEPLOY_USER}@${COZY_DEPLOY_SERVER}" "${COZY_DEPLOY_POSTSCRIPT}"
		else
			ssh "${COZY_DEPLOY_PROXY}" ssh "${COZY_DEPLOY_USER}@${COZY_DEPLOY_SERVER}" "${COZY_DEPLOY_POSTSCRIPT}"
		fi
	fi

	rm "${BINARY}"
	rm "${BINARY}.sha256"
}

do_clean() {
	find "${WORK_DIR}" -name "cozy-stack-*" -print -delete
}

check_env() {
	if [ "${COZY_ENV}" != "production" ] && [ "${COZY_ENV}" != "development" ]; then
		>&2 echo "ERR: COZY_ENV should either be production or development"
		exit 1
	fi
}

case "${1}" in
	release)
		do_release
		;;

	deploy)
		do_deploy
		;;

	clean)
		do_clean
		;;

	*)
		usage "${0}"
		exit 1
esac

exit 0
