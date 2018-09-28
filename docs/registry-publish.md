# Apps registry publication

## Automate publication

The following tutorial explains how to connect your continuous integration based
on Travis to automatically publish new versions on the apps registry.

In this tutorial, we assume:

-   you have a token allowing you to publish applications for your `editor`:
    `AbCdEf`
-   you are working on a repository plugged on travis and named on github
    `cozy/cozy-example`

You first need to add the token to your travis configuration file `.travis.yml`.
To do so, you need the
[`travis` utility](https://github.com/travis-ci/travis.rb#installation) to
encrypt its value.

```sh
$ travis encrypt REGISTRY_TOKEN=AbCdEf --add -r cozy/cozy-example
Please add the following to your .travis.yml file:

  secure: "jUAjkXNXfGIXyx2MKnnJ5HwCzhrQ29SaWBTmRpU6Rwi2XRieCnb2+MKZtjVcmEjfvJO38VjPozW2F4MYIxRXf9cD+ZRAEroZRcRSNHpoi/FJ6Ra767H7AbFDGpSSUSx7UDeZbSRNazCXJ55F/JaCq6F3XGeurrJbJ/tvMoIEvjg4qcOJpBgSxXEeyEnx5L3zbDoIqDo8hx9UtZoisiTC3TGq1CGFPe35VXnv/g23Uwg2Wux1drXXnMVghoVM8SDuoE9gf4LfppVHbYmowm25tylsvNKESbYiwJIkvPciPl2rABplJLJ4nuVpeWKHx1g+bChzlR5rhgXVJidua//yFD28xWS1+j+FhCGcYuPttYTntBVTiif0DVKS3gC1FFbf2ktgJVT7nYN2z0arhdPeK7Wtv8R+0SqlXUfBA/nam1pAS1xg2MTekVKxw+FmW0r6Ct4/Dta4d4XWsYiPMBrUOaCAqo+TkxBrVvM/LcM91ua33GKzMRLmKgbDY2k7lQpt3xA0Se02p4yiWcpN+3JzwVNRkuAQfw79ItJzhBP7ZTaQMwDByD/sN4ybhICWxTOLRh6kgfw+Xxv86aADvMVwfPcLljfk5Ot3kfLyaIyqrkIF9ePGSblt7RGzHiOECFr8qUtoGQAfekM+NmKzFSkeJU8t0EvHMen1NOsZhTemx9Q="
```

Like said, you need to add this block of ciphered data in the `.travis.yml`.
This will allow you to use the `REGISTRY_TOKEN` variable in your deployment
script.

Then you can adapt this script as your
[`after_deploy` or `after_success`](https://docs.travis-ci.com/user/customizing-the-build#The-Build-Lifecycle)
script.

It contains environment variables that you can adapt as your need:

-   `COZY_APP_VERSION`: the version string of the deployed version
-   `COZY_APP_PARAMETERS`: an optional JSON object (string, object or array)
    that will parameterize the application on its execution.
-   `COZY_BUILD_URL`: the URL of the deployed tarball for your application
-   `COZY_BUILD_BRANCH`: the name of the build branch from which the script
    creates dev releases

```bash
#!/bin/bash
set -e

# Environnment variables:
#   COZY_APP_VERSION: the version string of the deployed version
#   COZY_BUILD_URL: the URL of the deployed tarball for your application
#   COZY_BUILD_BRANCH: the name of the build branch from which the script
#                      creates dev releases

[ -z "${COZY_BUILD_BRANCH}" ] && COZY_BUILD_BRANCH="master"
[ -z "${COZY_APP_PARAMETERS}" ] && COZY_APP_PARAMETERS="null"

if [ "${TRAVIS_PULL_REQUEST}" != "false" ]; then
    echo "No deployment: in pull-request"
    exit 0
fi

if [ "${TRAVIS_BRANCH}" != "${COZY_BUILD_BRANCH}" ] && [ -z "${TRAVIS_TAG}" ]; then
    printf 'No deployment: not in %s branch nor tag (TRAVIS_BRANCH=%s TRAVIS_TAG=%s)\n' "${COZY_BUILD_BRANCH}" "${TRAVIS_BRANCH}" "${TRAVIS_TAG}"
    exit 0
fi

if ! jq <<< "${COZY_APP_PARAMETERS}" > /dev/null; then
  printf "Could not parse COZY_APP_PARAMETERS=%s as JSON\n" "${COZY_APP_PARAMETERS}"
  exit 1
fi

if [ -z "${COZY_APP_VERSION}" ]; then
    if [ -n "${TRAVIS_TAG}" ]; then
        COZY_APP_VERSION="${TRAVIS_TAG}"
    else
        manfile=$(find "${TRAVIS_BUILD_DIR}" \( -name "manifest.webapp" -o -name "manifest.konnector" \) | head -n1)
        COZY_APP_VERSION="$(jq -r '.version' < "${manfile}")-dev.${TRAVIS_COMMIT}"
    fi
fi

if [ -z "${COZY_BUILD_URL}" ]; then
    url="https://github.com/${TRAVIS_REPO_SLUG}/archive"
    if [ -n "${TRAVIS_TAG}" ]; then
        COZY_BUILD_URL="${url}/${TRAVIS_TAG}.tar.gz"
    else
        COZY_BUILD_URL="${url}/${TRAVIS_COMMIT}.tar.gz"
    fi
fi

shasum=$(curl -sSL --fail "${COZY_BUILD_URL}" | shasum -a 256 | cut -d" " -f1)

printf 'Publishing version "%s" from "%s" (%s)\n' "${COZY_APP_VERSION}" "${COZY_BUILD_URL}\n" "${shasum}"

curl -sS --fail -X POST \
    -H "Content-Type: application/json" \
    -H "Authorization: Token ${REGISTRY_TOKEN}" \
    -d "{\"version\": \"${COZY_APP_VERSION}\", \"url\": \"${COZY_BUILD_URL}\", \"sha256\": \"${shasum}\", \"parameters\": ${COZY_APP_PARAMETERS}}" \
    "https://registry.cozy.io/registry/versions"
```

## Access to our official apps registry

In order to access to our official repository, you need a token for a specific
editor. To do so, concact us directly at the address contact@cozycloud.cc with a
mail using the following title prefix: `[registry]` and precising the name of
the editor of your application.

We will provide you with the correct token.
