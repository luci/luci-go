// Copyright 2022 The LUCI Authors.
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

package cv

import (
	"context"
	"fmt"

	"google.golang.org/grpc"

	cvv0 "go.chromium.org/luci/cv/api/v0"
)

// FakeClient provides a fake implementation of cvv0.RunsClient for testing.
type FakeClient struct {
	Runs map[string]*cvv0.Run
}

func UseFakeClient(ctx context.Context, runs map[string]*cvv0.Run) context.Context {
	return context.WithValue(ctx, &fakeCVClientKey, &FakeClient{Runs: runs})
}

// GetRun mocks cvv0.RunsClient.GetRun RPC.
func (fc *FakeClient) GetRun(ctx context.Context, req *cvv0.GetRunRequest, opts ...grpc.CallOption) (*cvv0.Run, error) {
	if r, ok := fc.Runs[req.Id]; ok {
		return r, nil
	}
	return nil, fmt.Errorf("not found")
}
