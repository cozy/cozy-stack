#!/usr/bin/env bash
set -e

COZY_ENV_DFL=production

[ -z "${COZY_ENV}" ] && COZY_ENV="${COZY_ENV_DFL}"
[ -z "${COZY_DEPLOY_USER}" ] && COZY_DEPLOY_USER="${USER}"

pushd "$(dirname "$0")" > /dev/null
WORK_DIR=$(dirname "$(pwd)")
popd > /dev/null

if [ -r "${WORK_DIR}/local.env" ]; then
	. "${WORK_DIR}/local.env"
fi

echo_err() {
	>&2 echo -e "ERR: ${1}"
}

echo_wrn() {
	>&2 echo -e "WRN: ${1}"
}

usage() {
	echo -e "Usage: ${1} [release] [install] [dev] [assets] [debug-assets] [clean]"
	echo -e "\nCommands:\n"
	echo -e "  release       builds a release of the current working-tree"
	echo -e "  install       builds a release and install it the GOPATH"
	echo -e "  dev           builds a dev version"
	echo -e "  assets        move and download all the required assets (see: ./assets/externals)"
	echo -e "  debug-assets  create a debug-assets/ directory with all the assets"
	echo -e "  clean         remove all generated files from the working-tree"

	echo -e "\nEnvironment variables:"
	echo -e "\n  COZY_ENV"
	echo -e "    with release command, specify the environment of the release."
	echo -e "    can be \"production\" or \"development\". default: \"${COZY_ENV_DFL}\""
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
	#do_assets
	do_build

	openssl dgst -sha256 -hex "${BINARY}" > "${BINARY}.sha256"
	printf "%s\t" "${BINARY}"
	sed -E 's/SHA256\((.*)\)= ([a-f0-9]+)$/\2/g' < "${BINARY}.sha256"
}

do_install() {
	check_env
	do_prepare_ldflags

	printf "installing cozy-stack in %s... " "$(go env GOPATH)"
	go install -ldflags "\
		-X github.com/cozy/cozy-stack/pkg/config.Version=${VERSION_STRING} \
		-X github.com/cozy/cozy-stack/pkg/config.BuildTime=${BUILD_TIME} \
		-X github.com/cozy/cozy-stack/pkg/config.BuildMode=${BUILD_MODE}"
	echo "ok"
}

do_build() {
	check_env
	do_prepare_ldflags

	if [ -z "${1}" ]; then
		BINARY="$(pwd)/cozy-stack-${VERSION_OS_ARCH}-${VERSION_STRING}"
	else
		BINARY="${1}"
	fi

	printf "building cozy-stack in %s... " "${BINARY}"
	pushd "${WORK_DIR}" > /dev/null
	go build -ldflags "\
		-X github.com/cozy/cozy-stack/pkg/config.Version=${VERSION_STRING} \
		-X github.com/cozy/cozy-stack/pkg/config.BuildTime=${BUILD_TIME} \
		-X github.com/cozy/cozy-stack/pkg/config.BuildMode=${BUILD_MODE}" \
		-o "${BINARY}"
	popd > /dev/null
	echo "ok"
}

do_prepare_ldflags() {
	eval "$(go env)"

	VERSION_OS_ARCH="${GOOS}-${GOARCH}"
	if [ -z "${VERSION_STRING}" ]; then
		VERSION_STRING=$(git -C "${WORK_DIR}" --work-tree="${WORK_DIR}" \
			describe --tags --dirty 2> /dev/null)

		if [ -z "${VERSION_STRING}" ]; then
			VERSION_STRING="v0-$(git -C "${WORK_DIR}" rev-parse --short HEAD)"
			echo_wrn "No tag has been found to version the stack, using \"${VERSION_STRING}\" as version number"
		fi

		if ! git -C "${WORK_DIR}" diff --exit-code HEAD &>/dev/null; then
			if [ "${COZY_ENV}" == "production" ]; then
				echo_err "Can not build a production release in a dirty work-tree"
				exit 1
			fi
			VERSION_STRING="${VERSION_STRING}-dirty"
		fi

		if [ "${COZY_ENV}" == "development" ]; then
			VERSION_STRING="${VERSION_STRING}-dev"
		fi
	fi

	BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
	BUILD_MODE="${COZY_ENV}"
}

do_assets() {
	tx --root "${WORK_DIR}" pull -a || echo "Do you have configured transifex?"
	printf "executing go generate...\n"
	go get github.com/cozy/cozy-stack/pkg/statik
	pushd "${WORK_DIR}" > /dev/null
	go generate ./web
	popd > /dev/null
	echo "ok"
}

do_debug_assets() {
	tx --root "${WORK_DIR}" pull -a || echo "Do you have configured transifex?"
	assets_dst="${WORK_DIR}/debug-assets"
	rm -rf "${assets_dst}"
	mkdir -p "${assets_dst}/css" "${assets_dst}/js"
	pushd "$WORK_DIR" > /dev/null
	for i in assets/* ; do
		ln -s "${WORK_DIR}/${i}" "debug-${i}"
	done
	asset_name=""
	asset_url=""
	asset_sha=""
	while IFS= read -r line; do
		if [ "${line:0:1}" = "#" ]; then continue; fi
		if [ -z "${line}" ]; then continue; fi
		IFS=" " read -r -a line_split <<< "${line}"
		case "${line_split[0]}" in
			name)
				asset_name="${line_split[1]}"
				;;
			url)
				asset_url="${line_split[1]}"
				;;
			sha256)
				asset_sha="${line_split[1]}"
				download_debug_asset "${asset_name}" "${asset_url}" "${asset_sha}"
				asset_name=""
				asset_url=""
				asset_sha=""
				;;
			*)
				echo_err "Failed to parse assets/.externals file"
				echo_err "Unknown field named \"${line_split[0]}\""
				exit 1
				;;
			esac
	done < "assets/.externals"
	popd > /dev/null
	echo 'You can now run `go run . serve --assets ./debug-assets` to debug the assets'
}

download_debug_asset() {
	printf "downloading %s: " "${1}"
	if ! curl -sSL --fail "${2}" 1> "${assets_dst}/${1}" 2>/dev/null; then
		echo "failed"
		echo_err "Could not fetch resource with curl: ${2}"
		exit 1
	fi
	if [ -n "${3}" ]; then
		dgst=$(openssl dgst -sha256 < "${assets_dst}/${1}" | sed 's/^.* //')
		if [ "${3}" != "${dgst}" ]; then
			echo "failed"
			echo_err "Checksum SHA256 does not match for asset ${1} downloaded on ${2}:"
			echo_err "  expecting \"${dgst}\", got \"${3}\"."
			exit 1
		fi
	fi
	echo "ok"
}

do_clean() {
	find "${WORK_DIR}" -name "cozy-stack-*" -print -delete
}

check_env() {
	if [ "${COZY_ENV}" != "production" ] && [ "${COZY_ENV}" != "development" ]; then
		echo_err "ERR: COZY_ENV should either be production or development"
		exit 1
	fi
}

case "${1}" in
	release)
		do_release
		;;

	install)
		do_install
		;;

	clean)
		do_clean
		;;

	assets)
		do_assets
		;;

	debug-assets)
		do_debug_assets
		;;

	dev)
		COZY_ENV=development do_build cozy-stack
		;;

	*)
		usage "${0}"
		exit 1
esac

exit 0
