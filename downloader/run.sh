#!/bin/sh

set -e

# Downloads some version of the sdk-test-harness command, from the compiled binaries that are
# are published to GitHub, and runs it. You must specify either a full version string (v1.2.3)
# or a partial version (v1) in the environment variable VERSION, and any parameters you want to
# pass to the command in PARAMS.
#
# This script can be used in either Linux or MacOS; it will download whichever binary is
# appropriate for the current OS and architecture. It cannot be used in Windows. It requires
# /bin/sh and the commands, "grep", "sed", "curl", and "tar".

case "$(uname -s)" in
    Linux*)     OS_TYPE=Linux;;
    Darwin*)    OS_TYPE=Darwin;;
    CYGWIN*)    OS_TYPE=Windows;;
    MINGW*)     OS_TYPE=Windows;;
    MSYS_NT*)   OS_TYPE=Windows;;
    *)          OS_TYPE="UNKNOWN"
esac

if [ "${OS_TYPE}" = "UNKNOWN" ]; then
  echo 'Unrecognized or unsupport operating system. Supported Mac, Linux, Windows (MSYS, MINGW, CYGWIN)' >&2
  exit 1
fi

case "${OS_TYPE}" in
 Windows) EXTENSION="zip" ;;
 *) EXTENSION="tar.gz" ;;
esac

RELEASES_API_URL=https://api.github.com/repos/launchdarkly/sdk-test-harness/releases
RELEASES_SITE_URL=https://github.com/launchdarkly/sdk-test-harness/releases
EXECUTABLE_ARCHIVE_NAME="sdk-test-harness_${OS_TYPE}_$(uname -m).${EXTENSION}"

if [ -z "${VERSION}" -o -z "${PARAMS}" ]; then
  echo 'You must specify a version string in $VERSION and command parameters in $PARAMS' >&2
  exit 1
fi

resolve_version() {
  if echo "$1" | grep -q '^v[^.][^.]*\.[^.][^.]*\..'; then
    # It's already a complete version string
    echo "$1"
    exit
  fi
  curl -s "${RELEASES_API_URL}" \
    | grep "tag_name" \
    | sed -e 's/.*:[^"]*"\([^"]*\).*/\1/' \
    | grep "^$1\." \
    | head -n 1
}

VERSION_TO_DOWNLOAD=$(resolve_version "${VERSION}")
if [ -z "${VERSION_TO_DOWNLOAD}" ]; then
  echo "Unable to find a release matching '${VERSION}'" >&2
  exit 1
fi

TEMP_DIR="/tmp/sdk-test-harness_${VERSION_TO_DOWNLOAD}"
EXECUTABLE="${TEMP_DIR}/sdk-test-harness"
DOWNLOAD_URL="${RELEASES_SITE_URL}/download/${VERSION_TO_DOWNLOAD}/${EXECUTABLE_ARCHIVE_NAME}"

if [ ! -x "${EXECUTABLE}" ]; then
  rm -rf "${TEMP_DIR}"
  mkdir "${TEMP_DIR}"
  echo "Downloading ${DOWNLOAD_URL}"
  curl --fail -s -L -o "${TEMP_DIR}/archive.${EXTENSION}" "${DOWNLOAD_URL}" || (echo "Download failed" >&2; exit 1)
  if [ "${EXTENSION}" = "zip" ]; then
    unzip "${TEMP_DIR}/archive.${EXTENSION}" -d "${TEMP_DIR}"
  else
    tar -xf "${TEMP_DIR}/archive.${EXTENSION}" -C "${TEMP_DIR}"
  fi
fi

sh -c "${EXECUTABLE} $PARAMS"
