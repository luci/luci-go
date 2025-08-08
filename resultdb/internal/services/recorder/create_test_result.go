// Copyright 2019 The LUCI Authors.
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
	"strings"

	"go.chromium.org/luci/grpc/appstatus"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// CreateTestResult implements pb.RecorderServer.
func (s *recorderServer) CreateTestResult(ctx context.Context, in *pb.CreateTestResultRequest) (*pb.TestResult, error) {
	// Piggy back on BatchCreateTestResults.
	res, err := s.BatchCreateTestResults(ctx, &pb.BatchCreateTestResultsRequest{
		Parent:     in.Parent,
		Invocation: in.Invocation,
		Requests:   []*pb.CreateTestResultRequest{in},
		RequestId:  in.RequestId,
	})
	if err != nil {
		st, ok := appstatus.Get(err)

		// Attempt to fix up any references to requests[0]: in the batch response.
		if ok {
			msg := st.Message()
			if strings.HasPrefix(msg, "requests[0]: ") || strings.HasPrefix(msg, "bad request: requests[0]: ") {
				msg = strings.Replace(msg, "requests[0]: ", "", 1)
			}
			return nil, appstatus.Error(st.Code(), msg)
		}
		return nil, err
	}
	return res.TestResults[0], nil
}
