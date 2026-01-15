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

package rpc

import (
	"fmt"

	"google.golang.org/grpc/status"

	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

// StageAttemptCurrentState extracts *orchestratorpb.StageAttemptCurrentState
// from error details.
func StageAttemptCurrentState(err error) (*orchestratorpb.StageAttemptCurrentState, error) {
	if err == nil {
		return nil, nil
	}

	st, ok := status.FromError(err)
	if !ok {
		return nil, fmt.Errorf("err %w is not a grpc error", err)
	}

	details := st.Details()
	for _, d := range details {
		switch v := d.(type) {
		case *orchestratorpb.StageAttemptCurrentState:
			return v, nil
		}
	}
	return nil, nil
}
