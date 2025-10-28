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

package recorder

import (
	"context"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// FinalizeWorkUnit implements pb.RecorderServer.
func (s *recorderServer) FinalizeWorkUnit(ctx context.Context, in *pb.FinalizeWorkUnitRequest) (*pb.WorkUnit, error) {
	// Piggy back off BatchFinalizeWorkUnits.
	req := &pb.BatchFinalizeWorkUnitsRequest{
		Requests: []*pb.FinalizeWorkUnitRequest{in},
	}
	rsp, err := s.BatchFinalizeWorkUnits(ctx, req)
	if err != nil {
		// Remove any references to "requests[0]: ", this is a single create RPC not a batch RPC.
		return nil, removeRequestNumberFromAppStatusError(err)
	}
	return rsp.WorkUnits[0], nil
}
