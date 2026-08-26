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

package testresult

import (
	"context"
	"testing"

	"google.golang.org/grpc"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

type mockResultDBClient struct {
	pb.ResultDBClient
	getTestResult     func(ctx context.Context, in *pb.GetTestResultRequest) (*pb.TestResult, error)
	queryTestVerdicts func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error)
}

func (m *mockResultDBClient) GetTestResult(ctx context.Context, in *pb.GetTestResultRequest, opts ...grpc.CallOption) (*pb.TestResult, error) {
	if m.getTestResult != nil {
		return m.getTestResult(ctx, in)
	}
	return nil, errors.New("not implemented")
}

func (m *mockResultDBClient) QueryTestVerdicts(ctx context.Context, in *pb.QueryTestVerdictsRequest, opts ...grpc.CallOption) (*pb.QueryTestVerdictsResponse, error) {
	if m.queryTestVerdicts != nil {
		return m.queryTestVerdicts(ctx, in)
	}
	return &pb.QueryTestVerdictsResponse{}, nil
}

func TestCmdTestResult(t *testing.T) {
	t.Parallel()

	ftt.Run(`Cmd`, t, func(t *ftt.Test) {
		cmd := Cmd(nil)
		assert.Loosely(t, cmd, should.NotBeNil)
		assert.Loosely(t, cmd.UsageLine, should.Equal("test-result <subcommand>"))

		getCmd := GetCmd(nil)
		assert.Loosely(t, getCmd, should.NotBeNil)
		assert.Loosely(t, getCmd.UsageLine, should.Equal("get -invocationid <invocation_id> -testid <test_id> -resultid <result_id>"))
	})
}

func TestFetchTestResult(t *testing.T) {
	t.Parallel()

	ftt.Run(`FetchTestResult`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`legacy uses GetTestResult directly`, func(t *ftt.Test) {
			calledGet := false
			client := &mockResultDBClient{
				getTestResult: func(ctx context.Context, in *pb.GetTestResultRequest) (*pb.TestResult, error) {
					calledGet = true
					assert.Loosely(t, in.Name, should.Equal("invocations/build-123/tests/test1/results/res1"))
					return &pb.TestResult{
						Name:     in.Name,
						TestId:   "test1",
						ResultId: "res1",
						StatusV2: pb.TestResult_PASSED,
					}, nil
				},
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					t.Fatalf("QueryTestVerdicts should not be called in legacy mode")
					return nil, nil
				},
			}

			tr, err := FetchTestResult(ctx, client, "build-123", "test1", "res1", true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, calledGet, should.BeTrue)
			assert.Loosely(t, tr.ResultId, should.Equal("res1"))
		})

		t.Run(`root invocation uses QueryTestVerdicts directly`, func(t *ftt.Test) {
			calledQuery := false
			client := &mockResultDBClient{
				getTestResult: func(ctx context.Context, in *pb.GetTestResultRequest) (*pb.TestResult, error) {
					t.Fatalf("GetTestResult should not be called in root invocation mode")
					return nil, nil
				},
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					calledQuery = true
					assert.Loosely(t, in.Parent, should.Equal("rootInvocations/ants-i123"))
					return &pb.QueryTestVerdictsResponse{
						TestVerdicts: []*pb.TestVerdict{
							{
								TestId: "test1",
								Results: []*pb.TestResult{
									{
										Name:     "rootInvocations/ants-i123/workUnits/wu-1/tests/test1/results/res1",
										TestId:   "test1",
										ResultId: "res1",
										StatusV2: pb.TestResult_PASSED,
									},
								},
							},
						},
					}, nil
				},
			}

			tr, err := FetchTestResult(ctx, client, "ants-i123", "test1", "res1", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, calledQuery, should.BeTrue)
			assert.Loosely(t, tr.ResultId, should.Equal("res1"))
			assert.Loosely(t, tr.Name, should.Equal("rootInvocations/ants-i123/workUnits/wu-1/tests/test1/results/res1"))
		})
	})
}
