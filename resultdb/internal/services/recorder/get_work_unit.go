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
	"fmt"

	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/resultdb/internal/workunits"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// GetWorkUnit implements pb.RecorderServer.
//
// N.B. Unlike the API with the same name in ResultDB service, this API
// uses update tokens to authorise reads. It is intended for use by
// recorders only, specifically facilitating work unit updates using
// optimistic locking (using aip.dev/154 etags).
func (s *recorderServer) GetWorkUnit(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error) {
	wuID, err := workunits.ParseName(in.Name)
	if err != nil {
		return nil, appstatus.BadRequest(fmt.Errorf("name: %w", err))
	}

	// Piggy back on BatchGetWorkUnits.
	res, err := s.BatchGetWorkUnits(ctx, &pb.BatchGetWorkUnitsRequest{
		Parent: wuID.RootInvocationID.Name(),
		Names:  []string{in.Name},
		View:   in.View,
	})
	if err != nil {
		return nil, err
	}
	return res.WorkUnits[0], nil
}
