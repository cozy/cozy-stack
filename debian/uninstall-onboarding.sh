#!/usr/bin/env bash
export COZY_ADMIN_PASSWORD="$(cat /etc/cozy/.cozy-admin-passphrase)"

function app_installed {
	DOMAIN="${1}"
	APP="${2}"
	cozy-stack apps show --domain "${DOMAIN}" "${APP}" &>/dev/null
}

function uninstall_app {
	DOMAIN="${1}"
	APP="${2}"
	if app_installed "${@}"; then
		echo "    Uninstalling app ${APP}"
		cozy-stack apps uninstall --domain "${DOMAIN}" "${APP}" 1>/dev/null
	else
		echo "    App ${APP} already uninstalled, nothing to do"
	fi
}

echo "Uninstall onboarding"
cozy-stack instances ls | awk '{print $1}' | while read domain; do
		echo "  Migrating ${domain}"
		uninstall_app "${domain}" onboarding
done
