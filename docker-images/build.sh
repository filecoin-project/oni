#!/bin/bash

set -o errexit
set -o pipefail

set -e
set -x

err_report() {
    echo "Error on line $1"
}

trap 'err_report $LINENO' ERR

dir="$(dirname "$0")"

docker build -f "$dir/Dockerfile.oni-buildbase" "$dir"
docker build -f "$dir/Dockerfile.oni-runtime" "$dir"
