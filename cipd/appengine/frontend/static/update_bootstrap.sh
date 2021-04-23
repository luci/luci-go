#!/bin/bash

set -e

VERSION="5.0.0-alpha1"

curl "https://github.com/twbs/bootstrap/releases/download/v${VERSION}/bootstrap-${VERSION}-dist.zip" -L -o bootstrap.zip
rm -rf _bootstrap_staging
mkdir _bootstrap_staging
unzip bootstrap.zip -d _bootstrap_staging

BOOTSTRAP_DIR="_bootstrap_staging/bootstrap-${VERSION}-dist"

# Copy only stuff we need.
rm -rf bootstrap
mkdir -p bootstrap/css
cp ${BOOTSTRAP_DIR}/css/bootstrap.min.css bootstrap/css
cp ${BOOTSTRAP_DIR}/css/bootstrap.min.css.map bootstrap/css
mkdir -p bootstrap/js
cp ${BOOTSTRAP_DIR}/js/bootstrap.bundle.min.js bootstrap/js
cp ${BOOTSTRAP_DIR}/js/bootstrap.bundle.min.js.map bootstrap/js
echo ${VERSION} >> bootstrap/VERSION

# Cleanup staging files.
rm -rf _bootstrap_staging
rm bootstrap.zip
