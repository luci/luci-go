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

package artifact

import (
	"bytes"
	"context"
	"io"
	"os"
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
	listArtifacts  func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error)
	queryWorkUnits func(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error)
	getWorkUnit    func(ctx context.Context, in *pb.GetWorkUnitRequest) (*pb.WorkUnit, error)
}

func (m *mockResultDBClient) ListArtifacts(ctx context.Context, in *pb.ListArtifactsRequest, opts ...grpc.CallOption) (*pb.ListArtifactsResponse, error) {
	if m.listArtifacts != nil {
		return m.listArtifacts(ctx, in)
	}
	return &pb.ListArtifactsResponse{}, nil
}

func (m *mockResultDBClient) QueryWorkUnits(ctx context.Context, in *pb.QueryWorkUnitsRequest, opts ...grpc.CallOption) (*pb.QueryWorkUnitsResponse, error) {
	if m.queryWorkUnits != nil {
		return m.queryWorkUnits(ctx, in)
	}
	return &pb.QueryWorkUnitsResponse{}, nil
}

func (m *mockResultDBClient) GetWorkUnit(ctx context.Context, in *pb.GetWorkUnitRequest, opts ...grpc.CallOption) (*pb.WorkUnit, error) {
	if m.getWorkUnit != nil {
		return m.getWorkUnit(ctx, in)
	}
	return nil, errors.New("not implemented")
}

func captureStdout(fn func()) string {
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	outC := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		outC <- buf.String()
	}()
	fn()
	w.Close()
	os.Stdout = old
	return <-outC
}

func TestListCmd(t *testing.T) {
	t.Parallel()

	ftt.Run(`ListCmd`, t, func(t *ftt.Test) {
		trCmd := ListCmd(nil, ParentTypeTestResult)
		assert.Loosely(t, trCmd, should.NotBeNil)
		assert.Loosely(t, trCmd.ShortDesc, should.Equal("List artifacts for a test result"))
		assert.Loosely(t, trCmd.CommandRun().(*artifactListRun).maxArtifacts, should.Equal(100))

		wuCmd := ListCmd(nil, ParentTypeWorkUnit)
		assert.Loosely(t, wuCmd, should.NotBeNil)
		assert.Loosely(t, wuCmd.ShortDesc, should.Equal("List artifacts for a work unit"))
		assert.Loosely(t, wuCmd.CommandRun().(*artifactListRun).maxArtifacts, should.Equal(100))
	})
}

func TestQueryAllArtifacts(t *testing.T) {
	t.Parallel()

	ftt.Run(`QueryAllArtifacts`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`empty`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				listArtifacts: func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error) {
					return &pb.ListArtifactsResponse{}, nil
				},
			}
			arts, err := QueryAllArtifacts(ctx, client, "parent")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(arts), should.BeZero)
		})

		t.Run(`multi-page pagination`, func(t *ftt.Test) {
			calls := 0
			client := &mockResultDBClient{
				listArtifacts: func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error) {
					calls++
					if in.PageToken == "" {
						return &pb.ListArtifactsResponse{
							Artifacts: []*pb.Artifact{
								{ArtifactId: "art-1", Name: "parent/artifacts/art-1"},
								{ArtifactId: "art-2", Name: "parent/artifacts/art-2"},
							},
							NextPageToken: "token-page-2",
						}, nil
					}
					if in.PageToken == "token-page-2" {
						return &pb.ListArtifactsResponse{
							Artifacts: []*pb.Artifact{
								{ArtifactId: "art-3", Name: "parent/artifacts/art-3"},
							},
							NextPageToken: "",
						}, nil
					}
					return nil, errors.New("unexpected token")
				},
			}
			arts, err := QueryAllArtifacts(ctx, client, "parent")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(arts), should.Equal(3))
			assert.Loosely(t, calls, should.Equal(2))
			assert.Loosely(t, arts[0].ArtifactId, should.Equal("art-1"))
			assert.Loosely(t, arts[1].ArtifactId, should.Equal("art-2"))
			assert.Loosely(t, arts[2].ArtifactId, should.Equal("art-3"))
		})

		t.Run(`error propagation`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				listArtifacts: func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error) {
					return nil, errors.New("permission denied")
				},
			}
			_, err := QueryAllArtifacts(ctx, client, "parent")
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("permission denied"))
		})
	})
}

func TestQueryAncestorWorkUnits(t *testing.T) {
	t.Parallel()

	ftt.Run(`queryAncestorWorkUnits`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`successful query with AncestorsOf`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				queryWorkUnits: func(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error) {
					assert.Loosely(t, in.Parent, should.Equal("rootInvocations/build-123"))
					assert.Loosely(t, in.Predicate.AncestorsOf, should.Equal("rootInvocations/build-123/workUnits/step-1"))
					return &pb.QueryWorkUnitsResponse{
						WorkUnits: []*pb.WorkUnit{
							{Name: "rootInvocations/build-123/workUnits/root"},
							{Name: "rootInvocations/build-123/workUnits/parent-step"},
						},
					}, nil
				},
			}

			ancestors := queryAncestorWorkUnits(ctx, client, "rootInvocations/build-123/workUnits/step-1")
			assert.Loosely(t, len(ancestors), should.Equal(2))
			assert.Loosely(t, ancestors[0].Name, should.Equal("rootInvocations/build-123/workUnits/root"))
			assert.Loosely(t, ancestors[1].Name, should.Equal("rootInvocations/build-123/workUnits/parent-step"))
		})

		t.Run(`query error returns nil`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				queryWorkUnits: func(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error) {
					return nil, errors.New("QueryWorkUnits error")
				},
			}

			ancestors := queryAncestorWorkUnits(ctx, client, "rootInvocations/build-123/workUnits/child")
			assert.Loosely(t, len(ancestors), should.BeZero)
		})

		t.Run(`QueryWorkUnits with zero ancestors returns empty`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				queryWorkUnits: func(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error) {
					return &pb.QueryWorkUnitsResponse{WorkUnits: nil}, nil
				},
			}

			ancestors := queryAncestorWorkUnits(ctx, client, "rootInvocations/build-123/workUnits/root")
			assert.Loosely(t, len(ancestors), should.BeZero)
		})

		t.Run(`invalid target work unit format`, func(t *ftt.Test) {
			client := &mockResultDBClient{}
			ancestors := queryAncestorWorkUnits(ctx, client, "invalid")
			assert.Loosely(t, len(ancestors), should.BeZero)
		})
	})
}

func TestPrintAncestorWorkUnitNotices(t *testing.T) {
	t.Parallel()

	ftt.Run(`printAncestorWorkUnitNotices`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`test result with parent and ancestor artifacts`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				queryWorkUnits: func(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error) {
					return &pb.QueryWorkUnitsResponse{
						WorkUnits: []*pb.WorkUnit{
							{Name: "rootInvocations/build-123/workUnits/build-root"},
						},
					}, nil
				},
				listArtifacts: func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error) {
					if in.Parent == "rootInvocations/build-123/workUnits/step-1" {
						return &pb.ListArtifactsResponse{
							Artifacts: []*pb.Artifact{
								{ArtifactId: "stdout"},
								{ArtifactId: "stderr"},
							},
						}, nil
					}
					if in.Parent == "rootInvocations/build-123/workUnits/build-root" {
						return &pb.ListArtifactsResponse{
							Artifacts: []*pb.Artifact{
								{ArtifactId: "build-summary"},
							},
							NextPageToken: "more",
						}, nil
					}
					return &pb.ListArtifactsResponse{}, nil
				},
			}

			out := captureStdout(func() {
				printAncestorWorkUnitNotices(ctx, client, "rootInvocations/build-123/workUnits/step-1/tests/test1/results/0", ParentTypeTestResult)
			})

			assert.Loosely(t, out, should.ContainSubstring("Work Unit Artifacts:"))
			assert.Loosely(t, out, should.ContainSubstring("Work unit step-1 contains 2 artifacts. Run 'luci work-unit artifact list - step-1' to view."))
			assert.Loosely(t, out, should.ContainSubstring("Work unit build-root contains 1+ artifacts. Run 'luci work-unit artifact list - build-root' to view."))
		})

		t.Run(`work unit only checks ancestors, not target itself`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				queryWorkUnits: func(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error) {
					return &pb.QueryWorkUnitsResponse{
						WorkUnits: []*pb.WorkUnit{
							{Name: "rootInvocations/build-123/workUnits/step-1"}, // target itself
							{Name: "rootInvocations/build-123/workUnits/build-root"},
						},
					}, nil
				},
				listArtifacts: func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error) {
					if in.Parent == "rootInvocations/build-123/workUnits/build-root" {
						return &pb.ListArtifactsResponse{
							Artifacts: []*pb.Artifact{{ArtifactId: "root-log"}},
						}, nil
					}
					if in.Parent == "rootInvocations/build-123/workUnits/step-1" {
						t.Fatalf("target work unit itself should not be queried for ancestor notices")
					}
					return &pb.ListArtifactsResponse{}, nil
				},
			}

			out := captureStdout(func() {
				printAncestorWorkUnitNotices(ctx, client, "rootInvocations/build-123/workUnits/step-1", ParentTypeWorkUnit)
			})

			assert.Loosely(t, out, should.ContainSubstring("Work unit build-root contains 1 artifact. Run 'luci work-unit artifact list - build-root' to view."))
			assert.Loosely(t, out, should.NotContainSubstring("step-1"))
		})

		t.Run(`root work unit notice formatting`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				queryWorkUnits: func(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error) {
					return &pb.QueryWorkUnitsResponse{
						WorkUnits: []*pb.WorkUnit{
							{Name: "rootInvocations/build-123/workUnits/root"},
						},
					}, nil
				},
				listArtifacts: func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error) {
					if in.Parent == "rootInvocations/build-123/workUnits/root" {
						return &pb.ListArtifactsResponse{
							Artifacts: []*pb.Artifact{{ArtifactId: "root.log"}},
						}, nil
					}
					return &pb.ListArtifactsResponse{}, nil
				},
			}

			out := captureStdout(func() {
				printAncestorWorkUnitNotices(ctx, client, "rootInvocations/build-123/workUnits/step-1", ParentTypeWorkUnit)
			})

			assert.Loosely(t, out, should.ContainSubstring("Root work unit contains 1 artifact. Run 'luci work-unit artifact list - root' to view."))
		})

		t.Run(`no notice printed when ancestors have zero artifacts`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				queryWorkUnits: func(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error) {
					return &pb.QueryWorkUnitsResponse{
						WorkUnits: []*pb.WorkUnit{
							{Name: "rootInvocations/build-123/workUnits/parent"},
						},
					}, nil
				},
				listArtifacts: func(ctx context.Context, in *pb.ListArtifactsRequest) (*pb.ListArtifactsResponse, error) {
					return &pb.ListArtifactsResponse{}, nil
				},
			}

			out := captureStdout(func() {
				printAncestorWorkUnitNotices(ctx, client, "rootInvocations/build-123/workUnits/step/tests/t/results/0", ParentTypeTestResult)
			})

			assert.Loosely(t, out, should.Equal(""))
		})
	})
}
