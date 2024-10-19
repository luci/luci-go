// Copyright 2021 The LUCI Authors.
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

//go:build tools
// +build tools

package main

import (
	_ "github.com/envoyproxy/protoc-gen-validate"
	_ "github.com/golang/mock/mockgen"
	_ "github.com/golang/protobuf/protoc-gen-go"
	_ "github.com/smartystreets/goconvey"
	_ "golang.org/x/tools/cmd/goimports"
	_ "golang.org/x/tools/cmd/stringer"
	_ "google.golang.org/api/google-api-go-generator"
	_ "google.golang.org/grpc/cmd/protoc-gen-go-grpc"

	_ "go.chromium.org/luci/gae/tools/proto-gae"
	_ "go.chromium.org/luci/grpc/cmd/cproto"
	_ "go.chromium.org/luci/grpc/cmd/svcdec"
	_ "go.chromium.org/luci/grpc/cmd/svcmux"
	_ "go.chromium.org/luci/tools/cmd/apigen"
	_ "go.chromium.org/luci/tools/cmd/assets"
	_ "go.chromium.org/luci/tools/cmd/bqschemaupdater"

	_ "honnef.co/go/tools/cmd/staticcheck"
)
