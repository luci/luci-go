# Copyright 2025 The LUCI Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# This is a temporary script for building and uploading
# git-credential-luci with U2F support.
# The U2F support requires CGO which is not supported on builders yet,
# hence this temporary script.
# Builder CGO support is tracked on crbug.com/410936314
#
# You shouldn't run this script unless you know what you're doing
# (you won't have permission to upload).
set -eux

gver() {
    go version -m "$1"
}

# Uploads a built git-credential-luci file.
# Metadata is read from the built binary.
upload() {
    local -r file=$1
    local -r build_platform=linux-amd64
    local target_os=$(gver "$file" | awk -F= '/GOOS/ {print $2}')
    if [[ $target_os == darwin ]]; then
        target_os=mac
    fi
    local target_arch=$(gver "$file" | awk -F= '/GOARCH/ {print $2}')
    if [[ $target_arch == arm ]]; then
        local target_arm=$(gver "$file" | awk -F= '/GOARM/ {print $2}')
        if [[ $target_arm == 6 ]]; then
            target_arch=armv6l
        else
            exit 1
        fi
    fi
    local -r target_platform=$target_os-$target_arch
    local -r rev=$(gver "$file" | awk -F= '/vcs.revision/ {print $2}')
    local -r go_ver=$(go version "$file" | cut -d' ' -f2)

    tmp=$(mktemp -d)
    cp -t "$tmp" "$file"

    cipd create \
         -name "infra/tools/luci/git-credential-luci-custom-u2f/${target_platform}" \
         -ref latest \
         -tag "git_revision:$rev" \
         -tag "build_host_platform:${build_platform}" \
         -tag "go_version:${go_ver}" \
         -in "$tmp"

    rm -r "$tmp"
}

while read os arch; do
    if [[ $os == linux ]]; then
        flags="-tags=hidraw"
    else
        flags=
    fi
    if [[ $arch = armv6l ]]; then
        GOOS="$os" GOARCH=arm GOARM=6 go build "$flags"
    else
        GOOS="$os" GOARCH="$arch" go build "$flags"
    fi
    if [[ $os == windows ]]; then
       upload git-credential-luci.exe
    else
       upload git-credential-luci
    fi
done <<EOF
    darwin amd64
    darwin arm64
    linux amd64
    linux arm64
    windows amd64
    linux 386
    linux ppc64
    linux ppc64le
    linux riscv64
    linux s390x
    linux armv6l
    linux mips64
    linux mips64le
    linux mipsle
    linux loong64
    windows arm64
EOF
