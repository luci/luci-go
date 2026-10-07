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
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	"go.chromium.org/luci/logdog/api/logpb"
)

type mockLogsClient struct {
	logdog.LogsClient
	get  func(ctx context.Context, in *logdog.GetRequest, opts ...grpc.CallOption) (*logdog.GetResponse, error)
	tail func(ctx context.Context, in *logdog.TailRequest, opts ...grpc.CallOption) (*logdog.GetResponse, error)
}

func (m *mockLogsClient) Get(ctx context.Context, in *logdog.GetRequest, opts ...grpc.CallOption) (*logdog.GetResponse, error) {
	if m.get != nil {
		return m.get(ctx, in, opts...)
	}
	return nil, status.Errorf(codes.Unimplemented, "not implemented")
}

func (m *mockLogsClient) Tail(ctx context.Context, in *logdog.TailRequest, opts ...grpc.CallOption) (*logdog.GetResponse, error) {
	if m.tail != nil {
		return m.tail(ctx, in, opts...)
	}
	return nil, status.Errorf(codes.Unimplemented, "not implemented")
}

func makeTextEntry(streamIdx uint64, lines ...string) *logpb.LogEntry {
	pbLines := make([]*logpb.Text_Line, len(lines))
	for i, l := range lines {
		pbLines[i] = &logpb.Text_Line{
			Value:     []byte(l),
			Delimiter: "\n",
		}
	}
	return &logpb.LogEntry{
		StreamIndex: streamIdx,
		Content: &logpb.LogEntry_Text{
			Text: &logpb.Text{
				Lines: pbLines,
			},
		},
	}
}

func TestResolveTargetLog(t *testing.T) {
	t.Parallel()

	ftt.Run(`resolveTargetLog`, t, func(t *ftt.Test) {
		b := &pb.Build{
			Id: 8738491827364512345,
			Output: &pb.Build_Output{
				Logs: []*pb.Log{
					{Name: "stdout", Url: "logdog://logs.chromium.org/chromium/buildbucket/cr-buildbucket/8738491827364512345/+/stdout"},
				},
			},
			Steps: []*pb.Step{
				{
					Name:   "compile",
					Status: pb.Status_FAILURE,
					Logs: []*pb.Log{
						{Name: "stdout", Url: "logdog://logs.chromium.org/chromium/buildbucket/cr-buildbucket/8738491827364512345/+/u/compile/stdout"},
						{Name: "ninja_log", Url: "logdog://logs.chromium.org/chromium/buildbucket/cr-buildbucket/8738491827364512345/+/u/compile/ninja_log"},
					},
				},
				{
					Name:   "test|browser_tests|only_summary",
					Status: pb.Status_FAILURE,
					Logs: []*pb.Log{
						{Name: "failure_summary", Url: "logdog://logs.chromium.org/chromium/buildbucket/cr-buildbucket/8738491827364512345/+/u/test/summary"},
						{Name: "step_metadata", Url: "logdog://logs.chromium.org/chromium/buildbucket/cr-buildbucket/8738491827364512345/+/u/test/meta"},
					},
				},
				{
					Name:   "suiteA|ambiguous_leaf",
					Status: pb.Status_FAILURE,
					Logs:   []*pb.Log{{Name: "stdout", Url: "logdog://logs.chromium.org/chromium/a/+/stdout"}},
				},
				{
					Name:   "suiteB|ambiguous_leaf",
					Status: pb.Status_FAILURE,
					Logs:   []*pb.Log{{Name: "stdout", Url: "logdog://logs.chromium.org/chromium/b/+/stdout"}},
				},
				{
					Name:   "no_logs_step",
					Status: pb.Status_SUCCESS,
				},
			},
		}

		t.Run(`Exact step match and default stdout log`, func(t *ftt.Test) {
			l, err := resolveTargetLog(b, "compile", "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, l.Name, should.Equal("stdout"))
		})

		t.Run(`Exact step match and explicit log`, func(t *ftt.Test) {
			l, err := resolveTargetLog(b, "compile", "ninja_log")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, l.Name, should.Equal("ninja_log"))
		})

		t.Run(`Unique leaf step name fallback and failure_summary default`, func(t *ftt.Test) {
			l, err := resolveTargetLog(b, "only_summary", "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, l.Name, should.Equal("failure_summary"))
		})

		t.Run(`Ambiguous leaf step name returns error listing candidates`, func(t *ftt.Test) {
			_, err := resolveTargetLog(b, "ambiguous_leaf", "")
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("ambiguous"))
			assert.Loosely(t, err.Error(), should.ContainSubstring("suiteA|ambiguous_leaf"))
			assert.Loosely(t, err.Error(), should.ContainSubstring("suiteB|ambiguous_leaf"))
		})

		t.Run(`Step not found returns error with available steps`, func(t *ftt.Test) {
			_, err := resolveTargetLog(b, "nonexistent", "")
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring(`step "nonexistent" not found`))
			assert.Loosely(t, err.Error(), should.ContainSubstring("compile (FAILURE)"))
		})

		t.Run(`Step with no logs returns error`, func(t *ftt.Test) {
			_, err := resolveTargetLog(b, "no_logs_step", "")
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring(`has no logs`))
		})

		t.Run(`Log not found in step returns error with available logs`, func(t *ftt.Test) {
			_, err := resolveTargetLog(b, "compile", "missing_log")
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring(`log "missing_log" not found`))
			assert.Loosely(t, err.Error(), should.ContainSubstring("ninja_log"))
		})

		t.Run(`Build-level log when step is omitted`, func(t *ftt.Test) {
			l, err := resolveTargetLog(b, "", "stdout")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, l.Url, should.Equal("logdog://logs.chromium.org/chromium/buildbucket/cr-buildbucket/8738491827364512345/+/stdout"))
		})

		t.Run(`LogDog-sanitized step and log name fallback`, func(t *ftt.Test) {
			l, err := resolveTargetLog(b, "test", "summary")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, l.Name, should.Equal("failure_summary"))
		})
	})
}

func TestBuildLogRun(t *testing.T) {
	t.Parallel()

	ftt.Run(`buildLogRun`, t, func(t *ftt.Test) {
		af := base.NewAuthFlags()

		sampleBuild := &pb.Build{
			Id: 8738491827364512345,
			Steps: []*pb.Step{
				{
					Name:   "compile",
					Status: pb.Status_FAILURE,
					Logs: []*pb.Log{
						{
							Name: "stdout",
							Url:  "logdog://logs.chromium.org/chromium/buildbucket/cr-buildbucket/8738491827364512345/+/u/compile/stdout",
						},
					},
				},
			},
		}

		mockBB := &mockBuildsClient{
			getBuild: func(ctx context.Context, in *pb.GetBuildRequest, opts ...grpc.CallOption) (*pb.Build, error) {
				assert.Loosely(t, in.Id, should.Equal(int64(8738491827364512345)))
				return sampleBuild, nil
			},
		}

		t.Run(`Validation errors`, func(t *ftt.Test) {
			t.Run(`Missing -buildid returns 1`, func(t *ftt.Test) {
				cmd := LogGetCmd(af)
				run := cmd.CommandRun().(*buildLogRun)
				assert.Loosely(t, run.Flags.Parse([]string{"-step", "compile"}), should.BeNil)
				assert.Loosely(t, run.Run(nil, run.Flags.Args(), nil), should.Equal(1))
			})

			t.Run(`Missing -step and -log returns 1`, func(t *ftt.Test) {
				cmd := LogGetCmd(af)
				run := cmd.CommandRun().(*buildLogRun)
				assert.Loosely(t, run.Flags.Parse([]string{"-buildid", "8738491827364512345"}), should.BeNil)
				assert.Loosely(t, run.Run(nil, run.Flags.Args(), nil), should.Equal(1))
			})

			t.Run(`Unexpected positional arguments return 1`, func(t *ftt.Test) {
				cmd := LogGetCmd(af)
				run := cmd.CommandRun().(*buildLogRun)
				assert.Loosely(t, run.Flags.Parse([]string{"-buildid", "8738491827364512345", "-step", "compile", "extra"}), should.BeNil)
				assert.Loosely(t, run.Run(nil, run.Flags.Args(), nil), should.Equal(1))
			})

			t.Run(`Negative line count returns 1`, func(t *ftt.Test) {
				cmd := LogTailCmd(af)
				run := cmd.CommandRun().(*buildLogRun)
				assert.Loosely(t, run.Flags.Parse([]string{"-buildid", "8738491827364512345", "-step", "compile", "-n", "-5"}), should.BeNil)
				assert.Loosely(t, run.Run(nil, run.Flags.Args(), nil), should.Equal(1))
			})
		})

		t.Run(`List logs for build and specific step`, func(t *ftt.Test) {
			var out bytes.Buffer
			cmd := LogListCmd(af)
			run := cmd.CommandRun().(*buildLogListRun)
			run.buildsClient = mockBB
			run.out = &out

			assert.Loosely(t, run.Flags.Parse([]string{"-buildid", "8738491827364512345"}), should.BeNil)
			assert.Loosely(t, run.Run(nil, run.Flags.Args(), nil), should.Equal(0))
			assert.Loosely(t, out.String(), should.ContainSubstring("compile (FAILURE): stdout"))

			out.Reset()
			run2 := cmd.CommandRun().(*buildLogListRun)
			run2.buildsClient = mockBB
			run2.out = &out
			assert.Loosely(t, run2.Flags.Parse([]string{"-buildid", "8738491827364512345", "-step", "compile"}), should.BeNil)
			assert.Loosely(t, run2.Run(nil, run2.Flags.Args(), nil), should.Equal(0))
			assert.Loosely(t, out.String(), should.ContainSubstring("Step: compile (FAILURE)"))
			assert.Loosely(t, out.String(), should.ContainSubstring("stdout"))
		})

		t.Run(`Full log fetch across multiple LogEntries`, func(t *ftt.Test) {
			mockLD := &mockLogsClient{
				get: func(ctx context.Context, in *logdog.GetRequest, opts ...grpc.CallOption) (*logdog.GetResponse, error) {
					assert.Loosely(t, in.Project, should.Equal("chromium"))
					assert.Loosely(t, in.Path, should.Equal("buildbucket/cr-buildbucket/8738491827364512345/+/u/compile/stdout"))
					switch in.Index {
					case 0:
						return &logdog.GetResponse{
							State: &logdog.LogStreamState{TerminalIndex: 1},
							Desc:  &logpb.LogStreamDescriptor{StreamType: logpb.StreamType_TEXT},
							Logs: []*logpb.LogEntry{
								makeTextEntry(0, "line 1", "line 2"),
							},
						}, nil
					case 1:
						return &logdog.GetResponse{
							State: &logdog.LogStreamState{TerminalIndex: 1},
							Logs: []*logpb.LogEntry{
								makeTextEntry(1, "line 3", "line 4"),
							},
						}, nil
					default:
						return &logdog.GetResponse{}, nil
					}
				},
			}

			var out bytes.Buffer
			cmd := LogGetCmd(af)
			run := cmd.CommandRun().(*buildLogRun)
			run.buildsClient = mockBB
			run.logsClient = mockLD
			run.out = &out

			assert.Loosely(t, run.Flags.Parse([]string{"-buildid", "8738491827364512345", "-step", "compile"}), should.BeNil)
			ret := run.Run(nil, run.Flags.Args(), nil)
			assert.Loosely(t, ret, should.Equal(0))
			assert.Loosely(t, out.String(), should.Equal("line 1\nline 2\nline 3\nline 4\n"))
		})

		t.Run(`Head log fetch stops early once requested lines are satisfied`, func(t *ftt.Test) {
			getCalls := 0
			mockLD := &mockLogsClient{
				get: func(ctx context.Context, in *logdog.GetRequest, opts ...grpc.CallOption) (*logdog.GetResponse, error) {
					getCalls++
					return &logdog.GetResponse{
						State: &logdog.LogStreamState{TerminalIndex: 100},
						Desc:  &logpb.LogStreamDescriptor{StreamType: logpb.StreamType_TEXT},
						Logs: []*logpb.LogEntry{
							makeTextEntry(0, "line 1", "line 2", "line 3", "line 4"),
						},
					}, nil
				},
			}

			var out bytes.Buffer
			cmd := LogHeadCmd(af)
			run := cmd.CommandRun().(*buildLogRun)
			run.buildsClient = mockBB
			run.logsClient = mockLD
			run.out = &out

			assert.Loosely(t, run.Flags.Parse([]string{"-buildid", "8738491827364512345", "-step", "compile", "-n", "2"}), should.BeNil)
			ret := run.Run(nil, run.Flags.Args(), nil)
			assert.Loosely(t, ret, should.Equal(0))
			assert.Loosely(t, getCalls, should.Equal(1))
			assert.Loosely(t, out.String(), should.Equal("line 1\nline 2\n"))
		})

		t.Run(`Head and tail with -c alone ignore default 10-line limit`, func(t *ftt.Test) {
			twelveLines := []string{"L01", "L02", "L03", "L04", "L05", "L06", "L07", "L08", "L09", "L10", "L11", "L12"}
			mockLD := &mockLogsClient{
				get: func(ctx context.Context, in *logdog.GetRequest, opts ...grpc.CallOption) (*logdog.GetResponse, error) {
					return &logdog.GetResponse{
						State: &logdog.LogStreamState{TerminalIndex: 0},
						Desc:  &logpb.LogStreamDescriptor{StreamType: logpb.StreamType_TEXT},
						Logs: []*logpb.LogEntry{
							makeTextEntry(0, twelveLines...),
						},
					}, nil
				},
				tail: func(ctx context.Context, in *logdog.TailRequest, opts ...grpc.CallOption) (*logdog.GetResponse, error) {
					return &logdog.GetResponse{
						State: &logdog.LogStreamState{TerminalIndex: 0},
						Desc:  &logpb.LogStreamDescriptor{StreamType: logpb.StreamType_TEXT},
						Logs: []*logpb.LogEntry{
							makeTextEntry(0, twelveLines...),
						},
					}, nil
				},
			}

			// Each line is 4 bytes ("Lxx\n"), so 12 lines = 48 bytes.
			var headOut bytes.Buffer
			headCmd := LogHeadCmd(af)
			headRun := headCmd.CommandRun().(*buildLogRun)
			headRun.buildsClient = mockBB
			headRun.logsClient = mockLD
			headRun.out = &headOut
			assert.Loosely(t, headRun.Flags.Parse([]string{"-buildid", "8738491827364512345", "-step", "compile", "-c", "44"}), should.BeNil)
			assert.Loosely(t, headRun.Run(nil, headRun.Flags.Args(), nil), should.Equal(0))
			assert.Loosely(t, headOut.Len(), should.Equal(44))
			assert.Loosely(t, headOut.String(), should.ContainSubstring("L11\n"))

			var tailOut bytes.Buffer
			tailCmd := LogTailCmd(af)
			tailRun := tailCmd.CommandRun().(*buildLogRun)
			tailRun.buildsClient = mockBB
			tailRun.logsClient = mockLD
			tailRun.out = &tailOut
			assert.Loosely(t, tailRun.Flags.Parse([]string{"-buildid", "8738491827364512345", "-step", "compile", "-c", "44"}), should.BeNil)
			assert.Loosely(t, tailRun.Run(nil, tailRun.Flags.Args(), nil), should.Equal(0))
			assert.Loosely(t, tailOut.Len(), should.Equal(44))
			assert.Loosely(t, tailOut.String(), should.ContainSubstring("L02\n"))
			assert.Loosely(t, tailOut.String(), should.ContainSubstring("L12\n"))
		})

		t.Run(`Tail log fetch satisfied in single Tail RPC`, func(t *ftt.Test) {
			getCalled := false
			mockLD := &mockLogsClient{
				tail: func(ctx context.Context, in *logdog.TailRequest, opts ...grpc.CallOption) (*logdog.GetResponse, error) {
					return &logdog.GetResponse{
						State: &logdog.LogStreamState{TerminalIndex: 50},
						Desc:  &logpb.LogStreamDescriptor{StreamType: logpb.StreamType_TEXT},
						Logs: []*logpb.LogEntry{
							makeTextEntry(50, "line 48", "line 49", "line 50", "line 51"),
						},
					}, nil
				},
				get: func(ctx context.Context, in *logdog.GetRequest, opts ...grpc.CallOption) (*logdog.GetResponse, error) {
					getCalled = true
					return &logdog.GetResponse{}, nil
				},
			}

			var out bytes.Buffer
			cmd := LogTailCmd(af)
			run := cmd.CommandRun().(*buildLogRun)
			run.buildsClient = mockBB
			run.logsClient = mockLD
			run.out = &out

			assert.Loosely(t, run.Flags.Parse([]string{"-buildid", "8738491827364512345", "-step", "compile", "-n", "2"}), should.BeNil)
			ret := run.Run(nil, run.Flags.Args(), nil)
			assert.Loosely(t, ret, should.Equal(0))
			assert.Loosely(t, getCalled, should.BeFalse)
			assert.Loosely(t, out.String(), should.Equal("line 50\nline 51\n"))
		})

		t.Run(`Tail log fetch walks backwards via Get when Tail entry has fewer lines`, func(t *ftt.Test) {
			mockLD := &mockLogsClient{
				tail: func(ctx context.Context, in *logdog.TailRequest, opts ...grpc.CallOption) (*logdog.GetResponse, error) {
					return &logdog.GetResponse{
						State: &logdog.LogStreamState{TerminalIndex: 2},
						Desc:  &logpb.LogStreamDescriptor{StreamType: logpb.StreamType_TEXT},
						Logs: []*logpb.LogEntry{
							makeTextEntry(2, "line 5"),
						},
					}, nil
				},
				get: func(ctx context.Context, in *logdog.GetRequest, opts ...grpc.CallOption) (*logdog.GetResponse, error) {
					assert.Loosely(t, in.Index, should.Equal(int64(0)))
					assert.Loosely(t, in.LogCount, should.Equal(int32(2)))
					return &logdog.GetResponse{
						Logs: []*logpb.LogEntry{
							makeTextEntry(0, "line 1", "line 2"),
							makeTextEntry(1, "line 3", "line 4"),
						},
					}, nil
				},
			}

			var out bytes.Buffer
			cmd := LogTailCmd(af)
			run := cmd.CommandRun().(*buildLogRun)
			run.buildsClient = mockBB
			run.logsClient = mockLD
			run.out = &out

			assert.Loosely(t, run.Flags.Parse([]string{"-buildid", "8738491827364512345", "-step", "compile", "-n", "3"}), should.BeNil)
			ret := run.Run(nil, run.Flags.Args(), nil)
			assert.Loosely(t, ret, should.Equal(0))
			assert.Loosely(t, out.String(), should.Equal("line 3\nline 4\nline 5\n"))
		})

		t.Run(`Save log output to file with -o`, func(t *ftt.Test) {
			mockLD := &mockLogsClient{
				get: func(ctx context.Context, in *logdog.GetRequest, opts ...grpc.CallOption) (*logdog.GetResponse, error) {
					return &logdog.GetResponse{
						State: &logdog.LogStreamState{TerminalIndex: 0},
						Desc:  &logpb.LogStreamDescriptor{StreamType: logpb.StreamType_TEXT},
						Logs: []*logpb.LogEntry{
							makeTextEntry(0, "saved line 1", "saved line 2"),
						},
					}, nil
				},
			}

			tmpFile := filepath.Join(t.TempDir(), "compile.log")
			cmd := LogGetCmd(af)
			run := cmd.CommandRun().(*buildLogRun)
			run.buildsClient = mockBB
			run.logsClient = mockLD

			assert.Loosely(t, run.Flags.Parse([]string{"-buildid", "8738491827364512345", "-step", "compile", "-o", tmpFile}), should.BeNil)
			ret := run.Run(nil, run.Flags.Args(), nil)
			assert.Loosely(t, ret, should.Equal(0))

			content, err := os.ReadFile(tmpFile)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(content), should.Equal("saved line 1\nsaved line 2\n"))
		})

		t.Run(`Datagram stream returns error`, func(t *ftt.Test) {
			mockLD := &mockLogsClient{
				get: func(ctx context.Context, in *logdog.GetRequest, opts ...grpc.CallOption) (*logdog.GetResponse, error) {
					return &logdog.GetResponse{
						State: &logdog.LogStreamState{TerminalIndex: 0},
						Desc:  &logpb.LogStreamDescriptor{StreamType: logpb.StreamType_DATAGRAM},
					}, nil
				},
			}

			cmd := LogGetCmd(af)
			run := cmd.CommandRun().(*buildLogRun)
			run.buildsClient = mockBB
			run.logsClient = mockLD

			assert.Loosely(t, run.Flags.Parse([]string{"-buildid", "8738491827364512345", "-step", "compile"}), should.BeNil)
			ret := run.Run(nil, run.Flags.Args(), nil)
			assert.Loosely(t, ret, should.Equal(1))
		})
	})
}
