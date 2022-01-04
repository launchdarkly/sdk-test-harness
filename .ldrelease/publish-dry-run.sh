#!/bin/bash

set -eu

make build-release
cp dist/*.tar.gz dist/*.zip "${LD_RELEASE_ARTIFACTS_DIR}"
