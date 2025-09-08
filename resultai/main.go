// Copyright 2025 The LUCI Authors.
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

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/server/cfgmodule"
	luciserver "go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/module"

	"go.chromium.org/luci/resultai/llm"
	pb "go.chromium.org/luci/resultai/proto/v1"
	"go.chromium.org/luci/resultai/server"
)

const (
	ResultAiAccessGroup = "luci-resultai-access"
)

func authInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
	switch access, err := auth.IsMember(ctx, ResultAiAccessGroup); {
	case err != nil:
		return nil, status.Errorf(codes.Internal, "error when checking group membership - %s", err)
	case !access:
		return nil, status.Errorf(codes.PermissionDenied, "Current identity does not have access to ResultAI")
	}
	return handler(ctx, req)
}

func main() {
	modules := []module.Module{
		cfgmodule.NewModuleFromFlags(),
	}

	luciserver.Main(nil, modules, func(srv *luciserver.Server) error {
		geminiClient, err := llm.NewClient(srv.Context, srv.Options.CloudProject)
		if err != nil {
			return errors.Fmt("Failed to create GenAI client: %v", err)
		}
		// Exposed RPC services.
		pb.RegisterPromptServer(srv, &server.PromptServer{
			LlmClient: geminiClient,
		})

		// Auth check for resultai services.
		srv.RegisterUnaryServerInterceptors(authInterceptor)
		return nil
	})
}
