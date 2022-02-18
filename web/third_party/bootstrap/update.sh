#!/bin/bash

set -e

for VERSION in "5.1.3"
do
  MAJOR=$(echo ${VERSION} | cut -d "." -f 1)

  curl "https://github.com/twbs/bootstrap/releases/download/v${VERSION}/bootstrap-${VERSION}-dist.zip" -L -o bootstrap.zip
  rm -rf _staging
  mkdir _staging
  unzip bootstrap.zip -d _staging

  SRC_DIR="_staging/bootstrap-${VERSION}-dist"
  DST_DIR="v${MAJOR}"

  # Copy only stuff we need.
  rm -rf ${DST_DIR}
  mkdir -p ${DST_DIR}/css
  cp ${SRC_DIR}/css/bootstrap.min.css ${DST_DIR}/css
  cp ${SRC_DIR}/css/bootstrap.min.css.map ${DST_DIR}/css
  mkdir -p ${DST_DIR}/js
  cp ${SRC_DIR}/js/bootstrap.bundle.min.js ${DST_DIR}/js
  cp ${SRC_DIR}/js/bootstrap.bundle.min.js.map ${DST_DIR}/js
  echo ${VERSION} >> ${DST_DIR}/VERSION

  # Need a LICENSE.
  curl "https://raw.githubusercontent.com/twbs/bootstrap/main/LICENSE" -L -o ${DST_DIR}/LICENSE

  # Cleanup staging files.
  rm -rf _staging
  rm bootstrap.zip
done
