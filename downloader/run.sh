#!/bin/sh

set -e

# Downloads some version of the sdk-test-harness command, from the compiled binaries that are
# are published to GitHub, and runs it. You must specify either a full version string (v1.2.3)
# or a partial version (v1) in the environment variable VERSION, and any parameters you want to
# pass to the command in PARAMS.
#
# Sometimes you will hit Github rate limits when running this command. If you have a Github token,
# pass it in environment variable GITHUB_TOKEN; it is used both to list releases and to download
# the release asset, which raises the applicable rate limit.
#
# This script can be used in either Linux or MacOS; it will download whichever binary is
# appropriate for the current OS and architecture. It cannot be used in Windows. It requires
# /bin/sh and the commands, "grep", "sed", "curl", and "tar".

RELEASES_API_URL=https://api.github.com/repos/launchdarkly/sdk-test-harness/releases
RELEASES_SITE_URL=https://github.com/launchdarkly/sdk-test-harness/releases
EXECUTABLE_ARCHIVE_NAME=sdk-test-harness_$(uname -s)_$(uname -m).tar.gz

if [ -z "${VERSION}" -o -z "${PARAMS}" ]; then
  echo 'You must specify a version string in $VERSION and command parameters in $PARAMS' >&2
  exit 1
fi

# Github rate-limits requests, both to its APIs and to the release asset download endpoints. The
# effect is that sometimes the contract test step in CI fails spuriously. Authenticated requests get
# a substantially higher limit, so the token is used for every request that accepts it.
AUTH_TOKEN="${GITHUB_TOKEN}"
github_curl() {
  if [ -n "${AUTH_TOKEN}" ]; then
    curl -sS -L --retry 5 --retry-delay 2 -H "Authorization: Bearer ${AUTH_TOKEN}" "$@"
  else
    curl -sS -L --retry 5 --retry-delay 2 "$@"
  fi
}

resolve_version() {
  if echo "$1" | grep -q '^v[^.][^.]*\.[^.][^.]*\..'; then
    # It's already a complete version string
    echo "$1"
    return
  fi
  github_curl --fail "${RELEASES_API_URL}" \
    | grep "tag_name" \
    | sed -e 's/.*:[^"]*"\([^"]*\).*/\1/' \
    | grep "^$1\." \
    | head -n 1
}

# The unauthenticated github.com download URL drops the Authorization header when it redirects to
# the asset host, so an authenticated download has to go through the releases API instead.
resolve_asset_url() {
  github_curl --fail "${RELEASES_API_URL}/tags/$1" \
    | tr -d ' \n' \
    | tr '{' '\n' \
    | grep "\"name\":\"${EXECUTABLE_ARCHIVE_NAME}\"" \
    | grep -o "\"url\":\"[^\"]*/releases/assets/[0-9]*\"" \
    | sed -e 's/^"url":"//' -e 's/"$//' \
    | head -n 1
}

VERSION_TO_DOWNLOAD=$(resolve_version "${VERSION}")
if [ -z "${VERSION_TO_DOWNLOAD}" ]; then
  echo "Unable to find a release matching '${VERSION}'" >&2
  exit 1
fi

TEMP_DIR="/tmp/sdk-test-harness_${VERSION_TO_DOWNLOAD}"
EXECUTABLE="${TEMP_DIR}/sdk-test-harness"

if [ ! -x "${EXECUTABLE}" ]; then
  DOWNLOAD_URL="${RELEASES_SITE_URL}/download/${VERSION_TO_DOWNLOAD}/${EXECUTABLE_ARCHIVE_NAME}"
  DOWNLOAD_ACCEPT="*/*"
  if [ -n "${GITHUB_TOKEN}" ]; then
    ASSET_URL=$(resolve_asset_url "${VERSION_TO_DOWNLOAD}")
    if [ -n "${ASSET_URL}" ]; then
      DOWNLOAD_URL="${ASSET_URL}"
      DOWNLOAD_ACCEPT="application/octet-stream"
    else
      echo "Unable to find asset '${EXECUTABLE_ARCHIVE_NAME}' in release ${VERSION_TO_DOWNLOAD}; falling back to an unauthenticated download" >&2
      # The public download URL does not need credentials, and a rejected token would fail it.
      AUTH_TOKEN=""
    fi
  fi

  rm -rf "${TEMP_DIR}"
  mkdir "${TEMP_DIR}"
  echo "Downloading ${DOWNLOAD_URL}"
  github_curl --fail -H "Accept: ${DOWNLOAD_ACCEPT}" -o "${TEMP_DIR}/archive.tar.gz" "${DOWNLOAD_URL}" \
    || { echo "Download failed" >&2; exit 1; }
  tar -xf "${TEMP_DIR}/archive.tar.gz" -C "${TEMP_DIR}"
fi

sh -c "${EXECUTABLE} $PARAMS"
