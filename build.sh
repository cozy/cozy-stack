#!/bin/bash

COZY_ENV_DFL=production

[ -z ${COZY_ENV} ] && COZY_ENV=${COZY_ENV_DFL}

pushd `dirname $0` > /dev/null
WORK_DIR=`pwd`
popd > /dev/null

usage() {
	echo -e "Usage: ${1} [release]"
	echo -e "\nCommands:"
	echo -e "\trelease: builds a release of the current working-tree"

	echo -e "\nEnvironment variables:"
	echo -e "\tCOZY_ENV: with release command, specify the environment of the release: production or development. default: ${COZY_ENV_DFL}"
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
	set -e

	check_env

	VERSION_STRING=`git --git-dir=${WORK_DIR}/.git --work-tree=${WORK_DIR} \
		describe --tags --dirty 2> /dev/null | \
		sed -E 's/(.*)-g[[:xdigit:]]+(-?.*)$/\1\2/g'`

	if [ "$VERSION_STRING" == "" ]; then
		>&2 echo "WRN: No tag has been found to version the stack"
		if [ "${COZY_ENV}" == production ]; then
			>&2 echo "ERR: Can not build a production release without a tagged version"
			exit 1
		fi
		VERSION_STRING=v0-0-`git rev-parse --short HEAD`
	fi

	if [ `git diff --shortstat 2> /dev/null | tail -n1 | wc -l` -gt 0 ]; then
		if [ "${COZY_ENV}" == production ]; then
			>&2 echo "ERR: Can not build a production release in a dirty work-tree"
			exit 1
		fi
		VERSION_STRING="${VERSION_STRING}-dirty"
	fi

	if [ "${COZY_ENV}" == development ]; then
		VERSION_STRING="${VERSION_STRING}-dev"
	fi

	BINARY=cozy-stack-${VERSION_STRING}
	BUILD_TIME=`date -u +"%Y-%m-%dT%H:%M:%SZ"`
	BUILD_MODE=${COZY_ENV}

	go build \
		-ldflags "-X github.com/cozy/cozy-stack/config.Version=${VERSION_STRING}" \
		-ldflags "-X github.com/cozy/cozy-stack/config.BuildTime=${BUILD_TIME}" \
		-ldflags "-X github.com/cozy/cozy-stack/config.BuildMode=${BUILD_MODE}" \
		-o ${BINARY}

	openssl dgst -sha256 -hex ${BINARY} > ${BINARY}.sha256

	printf "${BINARY}\t"
	cat ${BINARY}.sha256 | sed -E 's/SHA256\((.*)\)= ([a-f0-9]+)$/\2/g'

	set +e
}

check_env() {
	if [ "${COZY_ENV}" != "production" ] && [ "${COZY_ENV}" != "development" ]; then
		>&2 echo "ERR: COZY_ENV should either be production or development"
		exit 1
	fi
}

case ${1} in
	release)
		do_release
		;;

	*)
		usage ${0}
		exit 1
esac

exit 0
