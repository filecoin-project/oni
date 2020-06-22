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

docker build -t "filecoin-project/oni-buildbase:latest" -f "$dir/Dockerfile.oni-buildbase" "$dir"
docker build -t "filecoin-project/oni-runtime:latest" -f "$dir/Dockerfile.oni-runtime" "$dir"
#docker push filecoin-project/oni-buildbase:"$COMMIT"
#docker push filecoin-project/oni-runtime:"$COMMIT"
