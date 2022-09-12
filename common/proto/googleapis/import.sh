#!/bin/bash

set -e

# List of proto files within https://github.com/googleapis/googleapis to import.
PROTOS_TO_IMPORT=(
  google/api/annotations.proto
  google/api/field_behavior.proto
  google/api/http.proto
  google/api/resource.proto

  google/cloud/security/privateca/v1/resources.proto

  google/rpc/code.proto
  google/rpc/error_details.proto
  google/rpc/status.proto

  google/type/color.proto
  google/type/date.proto
  google/type/dayofweek.proto
  google/type/expr.proto
  google/type/latlng.proto
  google/type/money.proto
  google/type/postal_address.proto
  google/type/timeofday.proto
)

cd $(dirname "${BASH_SOURCE[0]}")

# Clear existing imported files.
rm -rf google

# Grab the most recent checkout of "googleapis" repo.
CHECKOUT=".googleapis_checkout"
mkdir -p ${CHECKOUT}
pushd ${CHECKOUT}
git init
git remote rm origin || true
git remote add origin https://github.com/googleapis/googleapis
git config extensions.partialclone origin
git config remote.origin.fetch +refs/heads/master:refs/remotes/origin/master
git config remote.origin.partialclonefilter blob:none
git fetch --depth 1 origin
git checkout origin/master
REVISION=$(git rev-parse HEAD)
popd

# Copy all requested files + LICENSE + revision information.
for PROTO in ${PROTOS_TO_IMPORT[*]}
do
  mkdir -p $(dirname ${PROTO})
  cp "${CHECKOUT}/${PROTO}" ${PROTO}
done
cp "${CHECKOUT}/LICENSE" google/
echo "${REVISION}" > google/REVISION

# Make sure all dependencies are copied too by trying to compile everything.
set +e
echo
echo "Compiling all imported files to make sure their dependencies exist..."
echo
protoc --proto_path=. --descriptor_set_out=/dev/null ${PROTOS_TO_IMPORT[*]}
if [ $? -ne 0 ]; then
  echo
  echo "protoc call failed. Examine its output and add missing *.proto files"
  echo "to PROTOS_TO_IMPORT. Keep doing that until it no longer complains."
  echo
  echo "DO NOT IGNORE THIS ERROR!"
  exit 1
fi
echo "Success!"
