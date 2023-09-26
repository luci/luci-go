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

  # Expose fs.FS that contains bootstrap js and css.
  cat > ${DST_DIR}/embed.go << EOF
// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package bootstrap exposes a fs.FS variable that contains bootstrap js and css
package bootstrap

import "embed"

// FS is a file system that contains compiled bundled bootstrap js and css code
//
//go:embed css js
var FS embed.FS
EOF

  # Need a LICENSE.
  curl "https://raw.githubusercontent.com/twbs/bootstrap/main/LICENSE" -L -o ${DST_DIR}/LICENSE

  # Cleanup staging files.
  rm -rf _staging
  rm bootstrap.zip
done
