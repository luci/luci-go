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

package main

import (
	"context"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/cron"
	_ "go.chromium.org/luci/server/encryptedcookies/session/datastore"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/tq"
	_ "go.chromium.org/luci/server/tq/txn/datastore"
	pb "go.chromium.org/luci/tree_status/proto/v1"
)

func main() {
	// Additional modules that extend the server functionality.
	modules := []module.Module{
		cron.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
		tq.NewModuleFromFlags(),
	}

	server.Main(nil, modules, func(srv *server.Server) error {

		pb.RegisterTreeStatusServer(srv, &treeStatusServer{})

		return nil
	})
}

type treeStatusServer struct{}

func (*treeStatusServer) Get(ctx context.Context, request *pb.TreeStatusGetRequest) (*pb.TreeStatusGetResponse, error) {
	logging.Infof(ctx, "Get")
	return &pb.TreeStatusGetResponse{}, nil
}
