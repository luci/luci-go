// Copyright 2020 The LUCI Authors.
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

// Package main is the main entry point for the app.
package main

import (
	"go.chromium.org/luci/common/proto/access"
	"go.chromium.org/luci/server"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

func main() {
	server.Main(nil, nil, func(srv *server.Server) error {
		access.RegisterAccessServer(srv.PRPC, &access.UnimplementedAccessServer{})
		buildbucketpb.RegisterBuildsServer(srv.PRPC, &buildbucketpb.UnimplementedBuildsServer{})
		return nil
	})
}
