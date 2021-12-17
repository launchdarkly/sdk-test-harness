#!/bin/bash

set -eu

make publish-release
cp dist/*.tar.gz dist/*.zip "${LD_RELEASE_ARTIFACTS_DIR}"
