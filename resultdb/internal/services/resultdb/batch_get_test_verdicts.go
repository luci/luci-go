// Copyright 2026 The LUCI Authors.
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

package resultdb

import (
	"context"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/grpc/appstatus"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// BatchGetTestVerdicts implements the RPC method of the same name.
func (s *resultDBServer) BatchGetTestVerdicts(ctx context.Context, in *pb.BatchGetTestVerdictsRequest) (*pb.BatchGetTestVerdictsResponse, error) {
	return nil, appstatus.Error(codes.Unimplemented, "not implemented")
}
