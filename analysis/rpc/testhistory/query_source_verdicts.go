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

package testhistory

import (
	"context"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/grpc/appstatus"

	pb "go.chromium.org/luci/analysis/proto/v1"
)

// QuerySourceVerdicts queries source verdicts for a given test and source ref.
func (s *testHistoryServer) QuerySourceVerdicts(ctx context.Context, req *pb.QuerySourceVerdictsV2Request) (*pb.QuerySourceVerdictsV2Response, error) {
	return nil, appstatus.Error(codes.Unimplemented, "not implemented")
}
