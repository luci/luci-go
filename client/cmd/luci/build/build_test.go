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

package build

import (
	"bytes"
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "go.chromium.org/luci/buildbucket/proto"
	grpcpb "go.chromium.org/luci/buildbucket/proto/grpcpb"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type mockBuildsClient struct {
	grpcpb.BuildsClient
	getBuild func(ctx context.Context, in *pb.GetBuildRequest, opts ...grpc.CallOption) (*pb.Build, error)
}

func (m *mockBuildsClient) GetBuild(ctx context.Context, in *pb.GetBuildRequest, opts ...grpc.CallOption) (*pb.Build, error) {
	if m.getBuild != nil {
		return m.getBuild(ctx, in, opts...)
	}
	return nil, status.Errorf(codes.Unimplemented, "not implemented")
}

func TestFormatBuild(t *testing.T) {
	ftt.Run(`FormatBuild`, t, func(t *ftt.Test) {
		startTime := time.Date(2026, 9, 8, 7, 0, 0, 0, time.UTC)
		endTime := time.Date(2026, 9, 8, 7, 18, 30, 0, time.UTC)

		t.Run(`Successful build`, func(t *ftt.Test) {
			b := &pb.Build{
				Id:     8738491827364512345,
				Number: 12345,
				Builder: &pb.BuilderID{
					Project: "chromium",
					Bucket:  "ci",
					Builder: "linux-rel",
				},
				Status:     pb.Status_SUCCESS,
				CreateTime: timestamppb.New(startTime.Add(-10 * time.Second)),
				StartTime:  timestamppb.New(startTime),
				EndTime:    timestamppb.New(endTime),
				Input: &pb.Build_Input{
					GitilesCommit: &pb.GitilesCommit{
						Host:    "chromium.googlesource.com",
						Project: "chromium/src",
						Id:      "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
					},
					GerritChanges: []*pb.GerritChange{
						{
							Host:     "chromium-review.googlesource.com",
							Project:  "chromium/src",
							Change:   1234567,
							Patchset: 3,
						},
					},
				},
				Steps: []*pb.Step{
					{Name: "setup", Status: pb.Status_SUCCESS},
					{Name: "compile", Status: pb.Status_SUCCESS},
					{Name: "test", Status: pb.Status_SUCCESS},
				},
			}

			var buf bytes.Buffer
			err := FormatBuild(&buf, b, FormatOptions{})
			assert.Loosely(t, err, should.BeNil)
			out := buf.String()

			assert.Loosely(t, out, should.ContainSubstring("Build:         chromium/ci/linux-rel/12345"))
			assert.Loosely(t, out, should.ContainSubstring("Status:        SUCCESS"))
			assert.Loosely(t, out, should.ContainSubstring("Build ID:      8738491827364512345"))
			assert.Loosely(t, out, should.NotContainSubstring("URL:"))
			assert.Loosely(t, out, should.ContainSubstring("Timing:        18m 30s"))
			assert.Loosely(t, out, should.ContainSubstring("Commit:        https://chromium.googlesource.com/chromium/src/+/a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"))
			assert.Loosely(t, out, should.ContainSubstring("Gerrit CL:     https://crrev.com/c/1234567/3"))
			assert.Loosely(t, out, should.ContainSubstring("Invocation:    rootInvocations/build-8738491827364512345"))
			assert.Loosely(t, out, should.ContainSubstring("Steps:         3 passed"))
		})

		t.Run(`Failed build with failed steps and logs`, func(t *ftt.Test) {
			b := &pb.Build{
				Id:     8738491827364512346,
				Number: 12346,
				Builder: &pb.BuilderID{
					Project: "chromium",
					Bucket:  "ci",
					Builder: "linux-rel",
				},
				Status:          pb.Status_FAILURE,
				SummaryMarkdown: "Step 'compile' failed. See compile logs for details.",
				CreateTime:      timestamppb.New(startTime.Add(-5 * time.Second)),
				StartTime:       timestamppb.New(startTime),
				EndTime:         timestamppb.New(endTime),
				Steps: []*pb.Step{
					{Name: "setup", Status: pb.Status_SUCCESS},
					{
						Name:            "compile",
						Status:          pb.Status_FAILURE,
						StartTime:       timestamppb.New(startTime),
						EndTime:         timestamppb.New(startTime.Add(12 * time.Minute)),
						SummaryMarkdown: "ninja returned exit code 1",
						Logs: []*pb.Log{
							{Name: "stdout", ViewUrl: "https://logs.chromium.org/logs/1"},
							{Name: "ninja_log", ViewUrl: "https://logs.chromium.org/logs/2"},
						},
					},
					{Name: "archive", Status: pb.Status_CANCELED},
				},
			}

			var buf bytes.Buffer
			err := FormatBuild(&buf, b, FormatOptions{})
			assert.Loosely(t, err, should.BeNil)
			out := buf.String()

			assert.Loosely(t, out, should.ContainSubstring("Status:        FAILURE"))
			assert.Loosely(t, out, should.ContainSubstring("Summary:"))
			assert.Loosely(t, out, should.ContainSubstring("Step 'compile' failed. See compile logs for details."))
			assert.Loosely(t, out, should.ContainSubstring("Failed Steps (1 failed, 1 passed):"))
			assert.Loosely(t, out, should.ContainSubstring("✕ compile (FAILURE, 12m 00s)"))
			assert.Loosely(t, out, should.ContainSubstring("Summary: ninja returned exit code 1"))
			assert.Loosely(t, out, should.ContainSubstring("Logs: stdout (https://logs.chromium.org/logs/1), ninja_log (https://logs.chromium.org/logs/2)"))
			assert.Loosely(t, out, should.ContainSubstring("luci verdict list -invocationid build-8738491827364512346"))
			assert.Loosely(t, out, should.ContainSubstring("luci build get -buildid 8738491827364512346 -steps"))
		})

		t.Run(`Full step tree with AllSteps option`, func(t *ftt.Test) {
			b := &pb.Build{
				Id: 8738491827364512347,
				Steps: []*pb.Step{
					{Name: "test", Status: pb.Status_FAILURE},
					{Name: "test|browser_tests", Status: pb.Status_FAILURE},
					{Name: "test|browser_tests|run", Status: pb.Status_FAILURE, SummaryMarkdown: "Crash on startup"},
					{Name: "cleanup", Status: pb.Status_SUCCESS},
				},
			}

			var buf bytes.Buffer
			err := FormatBuild(&buf, b, FormatOptions{AllSteps: true})
			assert.Loosely(t, err, should.BeNil)
			out := buf.String()

			assert.Loosely(t, out, should.ContainSubstring("Steps (4 total: 1 passed, 3 failed, 0 running):"))
			assert.Loosely(t, out, should.ContainSubstring("[✕] test (FAILURE)"))
			assert.Loosely(t, out, should.ContainSubstring("  [✕] browser_tests (FAILURE)"))
			assert.Loosely(t, out, should.ContainSubstring("    [✕] run (FAILURE)"))
			assert.Loosely(t, out, should.ContainSubstring("    Summary: Crash on startup"))
			assert.Loosely(t, out, should.ContainSubstring("[✓] cleanup (SUCCESS)"))
		})

		t.Run(`Properties option`, func(t *ftt.Test) {
			inProps, _ := structpb.NewStruct(map[string]any{"os": "linux"})
			outProps, _ := structpb.NewStruct(map[string]any{"exit_code": 0})
			b := &pb.Build{
				Id: 8738491827364512348,
				Input: &pb.Build_Input{
					Properties: inProps,
				},
				Output: &pb.Build_Output{
					Properties: outProps,
				},
			}

			var buf bytes.Buffer
			err := FormatBuild(&buf, b, FormatOptions{Properties: true})
			assert.Loosely(t, err, should.BeNil)
			out := buf.String()

			assert.Loosely(t, out, should.ContainSubstring("Input Properties:"))
			assert.Loosely(t, out, should.ContainSubstring(`"os"`))
			assert.Loosely(t, out, should.ContainSubstring(`"linux"`))
			assert.Loosely(t, out, should.ContainSubstring("Output Properties:"))
			assert.Loosely(t, out, should.ContainSubstring(`"exit_code"`))
		})

		t.Run(`Cross-day timing formatting`, func(t *ftt.Test) {
			create := time.Date(2026, 9, 8, 23, 50, 0, 0, time.UTC)
			start := time.Date(2026, 9, 8, 23, 55, 0, 0, time.UTC)
			end := time.Date(2026, 9, 9, 1, 10, 0, 0, time.UTC)

			b := &pb.Build{
				Id:         8738491827364512349,
				Status:     pb.Status_SUCCESS,
				CreateTime: timestamppb.New(create),
				StartTime:  timestamppb.New(start),
				EndTime:    timestamppb.New(end),
			}

			var buf bytes.Buffer
			err := FormatBuild(&buf, b, FormatOptions{})
			assert.Loosely(t, err, should.BeNil)
			out := buf.String()

			assert.Loosely(t, out, should.ContainSubstring("Timing:        1h 15m 00s (Created: 2026-09-08 23:50:00 UTC, Started: 23:55:00, Ended: 2026-09-09 01:10:00)"))
		})
	})
}

func TestBuildGetRun(t *testing.T) {
	ftt.Run(`buildGetRun`, t, func(t *ftt.Test) {
		af := base.NewAuthFlags()

		t.Run(`Missing target returns 1`, func(t *ftt.Test) {
			cmd := GetCmd(af)
			ret := cmd.CommandRun().Run(nil, []string{}, nil)
			assert.Loosely(t, ret, should.Equal(1))
		})

		t.Run(`Positional arguments return 1`, func(t *ftt.Test) {
			cmd := GetCmd(af)
			ret := cmd.CommandRun().Run(nil, []string{"8738491827364512345"}, nil)
			assert.Loosely(t, ret, should.Equal(1))
		})

		t.Run(`Invalid -buildid returns error on parse`, func(t *ftt.Test) {
			cmd := GetCmd(af)
			run := cmd.CommandRun().(*buildGetRun)
			err := run.Flags.Parse([]string{"-buildid", "not-a-number"})
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run(`Top-level build command routing`, func(t *ftt.Test) {
			topCmd := Cmd(af)
			assert.Loosely(t, topCmd.UsageLine, should.Equal("build <subcommand>"))
			run := topCmd.CommandRun()
			assert.Loosely(t, run.Run(nil, []string{}, nil), should.Equal(0))
			assert.Loosely(t, run.Run(nil, []string{"--help"}, nil), should.Equal(0))
			assert.Loosely(t, run.Run(nil, []string{"get", "--help"}, nil), should.Equal(0))
		})

		t.Run(`Execution with mock client - buildid flag`, func(t *ftt.Test) {
			mock := &mockBuildsClient{
				getBuild: func(ctx context.Context, in *pb.GetBuildRequest, opts ...grpc.CallOption) (*pb.Build, error) {
					assert.Loosely(t, in.Id, should.Equal(8738491827364512345))
					assert.Loosely(t, in.Mask, should.NotBeNil)
					return &pb.Build{
						Id:     8738491827364512345,
						Status: pb.Status_SUCCESS,
						Builder: &pb.BuilderID{
							Project: "chromium",
							Bucket:  "ci",
							Builder: "linux-rel",
						},
					}, nil
				},
			}

			cmd := GetCmd(af)
			run := cmd.CommandRun().(*buildGetRun)
			run.client = mock
			err := run.Flags.Parse([]string{"-buildid", "8738491827364512345"})
			assert.Loosely(t, err, should.BeNil)
			ret := run.Run(nil, run.Flags.Args(), nil)
			assert.Loosely(t, ret, should.Equal(0))
		})

		t.Run(`Execution with mock client - id alias`, func(t *ftt.Test) {
			mock := &mockBuildsClient{
				getBuild: func(ctx context.Context, in *pb.GetBuildRequest, opts ...grpc.CallOption) (*pb.Build, error) {
					assert.Loosely(t, in.Id, should.Equal(8738491827364512345))
					return &pb.Build{
						Id:     8738491827364512345,
						Status: pb.Status_SUCCESS,
					}, nil
				},
			}

			cmd := GetCmd(af)
			run := cmd.CommandRun().(*buildGetRun)
			run.client = mock
			err := run.Flags.Parse([]string{"-id", "8738491827364512345"})
			assert.Loosely(t, err, should.BeNil)
			ret := run.Run(nil, run.Flags.Args(), nil)
			assert.Loosely(t, ret, should.Equal(0))
		})

		t.Run(`Execution with mock client - json flag`, func(t *ftt.Test) {
			mock := &mockBuildsClient{
				getBuild: func(ctx context.Context, in *pb.GetBuildRequest, opts ...grpc.CallOption) (*pb.Build, error) {
					assert.Loosely(t, in.Id, should.Equal(8738491827364512345))
					assert.Loosely(t, in.Mask.AllFields, should.BeTrue)
					return &pb.Build{
						Id:     8738491827364512345,
						Status: pb.Status_SUCCESS,
					}, nil
				},
			}

			cmd := GetCmd(af)
			run := cmd.CommandRun().(*buildGetRun)
			run.client = mock
			err := run.Flags.Parse([]string{"-buildid", "8738491827364512345", "-json"})
			assert.Loosely(t, err, should.BeNil)
			ret := run.Run(nil, run.Flags.Args(), nil)
			assert.Loosely(t, ret, should.Equal(0))
		})

		t.Run(`Execution with mock client - RPC error returns 1`, func(t *ftt.Test) {
			mock := &mockBuildsClient{
				getBuild: func(ctx context.Context, in *pb.GetBuildRequest, opts ...grpc.CallOption) (*pb.Build, error) {
					return nil, status.Errorf(codes.NotFound, "build not found")
				},
			}

			cmd := GetCmd(af)
			run := cmd.CommandRun().(*buildGetRun)
			run.client = mock
			err := run.Flags.Parse([]string{"-buildid", "8738491827364512345"})
			assert.Loosely(t, err, should.BeNil)
			ret := run.Run(nil, run.Flags.Args(), nil)
			assert.Loosely(t, ret, should.Equal(1))
		})
	})
}
