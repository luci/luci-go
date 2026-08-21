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

package format

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestParseTradefedXMLReason(t *testing.T) {
	t.Parallel()

	ftt.Run(`ParseTradefedXMLReason`, t, func(t *ftt.Test) {
		t.Run(`XML with Reason tag and attributes`, func(t *ftt.Test) {
			xmlStr := `<?xml version="1.0" encoding="utf-8"?>
<TestResult>
  <TestRun name="test_run_1">
    <Reason message="Failed initialization Stack:java.lang.NoClassDefFoundError: Foo&#13;&#10;&#9;at bar.Baz(Baz.java:10)" error_name="INSTRUMENTATION_NULL_METHOD" error_code="530002" />
  </TestRun>
</TestResult>`
			r, err := ParseTradefedXMLReason(strings.NewReader(xmlStr))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, r, should.NotBeNil)
			assert.Loosely(t, r.ErrorName, should.Equal("INSTRUMENTATION_NULL_METHOD"))
			assert.Loosely(t, r.ErrorCode, should.Equal("530002"))

			msg, stack := cleanAndSplitReasonMessage(r.Message)
			assert.Loosely(t, msg, should.Equal("Failed initialization"))
			assert.Loosely(t, stack, should.Equal("java.lang.NoClassDefFoundError: Foo\n  at bar.Baz(Baz.java:10)"))
		})

		t.Run(`XML without Reason tag`, func(t *ftt.Test) {
			xmlStr := `<?xml version="1.0"?><TestResult><Test name="test1" /></TestResult>`
			r, err := ParseTradefedXMLReason(strings.NewReader(xmlStr))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, r, should.BeNil)
		})
	})
}

type fakeDiscoveryRDBClient struct {
	pb.ResultDBClient
	getWorkUnitFunc       func(ctx context.Context, in *pb.GetWorkUnitRequest, opts ...grpc.CallOption) (*pb.WorkUnit, error)
	queryWorkUnitsFunc    func(ctx context.Context, in *pb.QueryWorkUnitsRequest, opts ...grpc.CallOption) (*pb.QueryWorkUnitsResponse, error)
	listArtifactsFunc     func(ctx context.Context, in *pb.ListArtifactsRequest, opts ...grpc.CallOption) (*pb.ListArtifactsResponse, error)
	getRootInvocationFunc func(ctx context.Context, in *pb.GetRootInvocationRequest, opts ...grpc.CallOption) (*pb.RootInvocation, error)
	getInvocationFunc     func(ctx context.Context, in *pb.GetInvocationRequest, opts ...grpc.CallOption) (*pb.Invocation, error)
}

func (f *fakeDiscoveryRDBClient) GetWorkUnit(ctx context.Context, in *pb.GetWorkUnitRequest, opts ...grpc.CallOption) (*pb.WorkUnit, error) {
	if f.getWorkUnitFunc != nil {
		return f.getWorkUnitFunc(ctx, in, opts...)
	}
	return nil, nil
}

func (f *fakeDiscoveryRDBClient) QueryWorkUnits(ctx context.Context, in *pb.QueryWorkUnitsRequest, opts ...grpc.CallOption) (*pb.QueryWorkUnitsResponse, error) {
	if f.queryWorkUnitsFunc != nil {
		return f.queryWorkUnitsFunc(ctx, in, opts...)
	}
	return nil, errors.New("unimplemented")
}

func (f *fakeDiscoveryRDBClient) ListArtifacts(ctx context.Context, in *pb.ListArtifactsRequest, opts ...grpc.CallOption) (*pb.ListArtifactsResponse, error) {
	if f.listArtifactsFunc != nil {
		return f.listArtifactsFunc(ctx, in, opts...)
	}
	return &pb.ListArtifactsResponse{}, nil
}

func (f *fakeDiscoveryRDBClient) GetRootInvocation(ctx context.Context, in *pb.GetRootInvocationRequest, opts ...grpc.CallOption) (*pb.RootInvocation, error) {
	if f.getRootInvocationFunc != nil {
		return f.getRootInvocationFunc(ctx, in, opts...)
	}
	return nil, nil
}

func (f *fakeDiscoveryRDBClient) GetInvocation(ctx context.Context, in *pb.GetInvocationRequest, opts ...grpc.CallOption) (*pb.Invocation, error) {
	if f.getInvocationFunc != nil {
		return f.getInvocationFunc(ctx, in, opts...)
	}
	return nil, nil
}

func TestDiscoverWorkUnitError(t *testing.T) {
	t.Parallel()

	ftt.Run(`DiscoverWorkUnitError`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`Layer 1: Work Unit SummaryMarkdown`, func(t *ftt.Test) {
			fakeClient := &fakeDiscoveryRDBClient{
				getWorkUnitFunc: func(ctx context.Context, in *pb.GetWorkUnitRequest, opts ...grpc.CallOption) (*pb.WorkUnit, error) {
					return &pb.WorkUnit{
						Name:            in.Name,
						SummaryMarkdown: "### Work Unit Failed: Runner crashed",
					}, nil
				},
			}
			err, dErr := DiscoverWorkUnitError(ctx, fakeClient, nil, "rootInvocations/inv1/workUnits/wu1")
			assert.Loosely(t, dErr, should.BeNil)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.RawSummary, should.Equal("### Work Unit Failed: Runner crashed"))
		})

		t.Run(`Layer 2: Tradefed XML Artifact`, func(t *ftt.Test) {
			var gzBuf bytes.Buffer
			gw := gzip.NewWriter(&gzBuf)
			gw.Write([]byte(`<?xml version="1.0"?><TestResult><Reason message="Module failed Stack:java.lang.Exception: boom" error_name="DEVICE_UNRESPONSIVE" error_code="1001" /></TestResult>`))
			gw.Close()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write(gzBuf.Bytes())
			}))
			defer server.Close()

			fakeClient := &fakeDiscoveryRDBClient{
				getWorkUnitFunc: func(ctx context.Context, in *pb.GetWorkUnitRequest, opts ...grpc.CallOption) (*pb.WorkUnit, error) {
					return &pb.WorkUnit{Name: in.Name}, nil
				},
				listArtifactsFunc: func(ctx context.Context, in *pb.ListArtifactsRequest, opts ...grpc.CallOption) (*pb.ListArtifactsResponse, error) {
					return &pb.ListArtifactsResponse{
						Artifacts: []*pb.Artifact{
							{
								ArtifactId: "subprocess-test_result.xml_123.xml.gz",
								FetchUrl:   server.URL,
							},
						},
					}, nil
				},
			}

			err, dErr := DiscoverWorkUnitError(ctx, fakeClient, server.Client(), "rootInvocations/inv1/workUnits/wu1")
			assert.Loosely(t, dErr, should.BeNil)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.ErrorName, should.Equal("DEVICE_UNRESPONSIVE"))
			assert.Loosely(t, err.ErrorCode, should.Equal("1001"))
			assert.Loosely(t, err.Message, should.Equal("Module failed"))
			assert.Loosely(t, err.StackTrace, should.Equal("java.lang.Exception: boom"))
		})

		t.Run(`Layer 3: Root Invocation SummaryMarkdown`, func(t *ftt.Test) {
			fakeClient := &fakeDiscoveryRDBClient{
				getWorkUnitFunc: func(ctx context.Context, in *pb.GetWorkUnitRequest, opts ...grpc.CallOption) (*pb.WorkUnit, error) {
					return &pb.WorkUnit{Name: in.Name}, nil
				},
				getRootInvocationFunc: func(ctx context.Context, in *pb.GetRootInvocationRequest, opts ...grpc.CallOption) (*pb.RootInvocation, error) {
					return &pb.RootInvocation{
						Name:            in.Name,
						SummaryMarkdown: "Harness timeout after 3600s",
					}, nil
				},
			}

			err, dErr := DiscoverWorkUnitError(ctx, fakeClient, nil, "rootInvocations/inv1/workUnits/wu1")
			assert.Loosely(t, dErr, should.BeNil)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.RawSummary, should.Equal("Harness timeout after 3600s"))
		})
	})
}

func TestFormatDiscoveredErrorFirstLine(t *testing.T) {
	t.Parallel()

	ftt.Run(`FormatDiscoveredErrorFirstLine`, t, func(t *ftt.Test) {
		s0, t0 := FormatDiscoveredErrorFirstLine(nil, 100)
		assert.Loosely(t, s0, should.Equal(""))
		assert.Loosely(t, t0, should.BeFalse)

		err1 := &DiscoveredError{
			ErrorName: "MY_ERROR",
			ErrorCode: "123",
			Message:   "First line\nSecond line",
		}
		s1, t1 := FormatDiscoveredErrorFirstLine(err1, 100)
		assert.Loosely(t, s1, should.Equal("[MY_ERROR|123] First line"))
		assert.Loosely(t, t1, should.BeTrue)

		err2 := &DiscoveredError{
			RawSummary: "### Error headline\nExtra details",
		}
		s2, t2 := FormatDiscoveredErrorFirstLine(err2, 100)
		assert.Loosely(t, s2, should.Equal("### Error headline"))
		assert.Loosely(t, t2, should.BeTrue)

		errSingle := &DiscoveredError{
			ErrorName: "ERR",
			Message:   "Single line",
		}
		s3, t3 := FormatDiscoveredErrorFirstLine(errSingle, 100)
		assert.Loosely(t, s3, should.Equal("[ERR] Single line"))
		assert.Loosely(t, t3, should.BeFalse)
	})
}

func TestDiscoveryCache(t *testing.T) {
	t.Parallel()

	ftt.Run(`DiscoveryCache`, t, func(t *ftt.Test) {
		ctx := WithDiscoveryCache(context.Background())

		getWUCount := 0
		listArtCount := 0
		httpFetchCount := 0

		var gzBuf bytes.Buffer
		gw := gzip.NewWriter(&gzBuf)
		gw.Write([]byte(`<?xml version="1.0"?><TestResult><Reason message="Module failed" error_name="ERROR" error_code="1" /></TestResult>`))
		gw.Close()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			httpFetchCount++
			w.WriteHeader(http.StatusOK)
			w.Write(gzBuf.Bytes())
		}))
		defer server.Close()

		fakeClient := &fakeDiscoveryRDBClient{
			queryWorkUnitsFunc: func(ctx context.Context, in *pb.QueryWorkUnitsRequest, opts ...grpc.CallOption) (*pb.QueryWorkUnitsResponse, error) {
				getWUCount++
				return &pb.QueryWorkUnitsResponse{
					WorkUnits: []*pb.WorkUnit{
						{Name: "rootInvocations/inv1/workUnits/parent_wu", Parent: "rootInvocations/inv1"},
					},
				}, nil
			},
			getWorkUnitFunc: func(ctx context.Context, in *pb.GetWorkUnitRequest, opts ...grpc.CallOption) (*pb.WorkUnit, error) {
				getWUCount++
				return &pb.WorkUnit{Name: in.Name, Parent: "rootInvocations/inv1/workUnits/parent_wu"}, nil
			},
			listArtifactsFunc: func(ctx context.Context, in *pb.ListArtifactsRequest, opts ...grpc.CallOption) (*pb.ListArtifactsResponse, error) {
				if in.Parent == "rootInvocations/inv1/workUnits/parent_wu" {
					listArtCount++
					return &pb.ListArtifactsResponse{
						Artifacts: []*pb.Artifact{
							{
								ArtifactId: "subprocess-test_result.xml.gz",
								FetchUrl:   server.URL,
							},
						},
					}, nil
				}
				return &pb.ListArtifactsResponse{}, nil
			},
		}

		// First call
		err1, dErr1 := DiscoverWorkUnitError(ctx, fakeClient, server.Client(), "rootInvocations/inv1/workUnits/child_wu1")
		assert.Loosely(t, dErr1, should.BeNil)
		assert.Loosely(t, err1, should.NotBeNil)
		assert.Loosely(t, err1.Message, should.Equal("Module failed"))

		initialWUCount := getWUCount
		initialParentArtCount := listArtCount
		initialHTTPCount := httpFetchCount

		// Second call on exact same work unit should be 100% cache hit
		err2, dErr2 := DiscoverWorkUnitError(ctx, fakeClient, server.Client(), "rootInvocations/inv1/workUnits/child_wu1")
		assert.Loosely(t, dErr2, should.BeNil)
		assert.Loosely(t, err2, should.Equal(err1))
		assert.Loosely(t, getWUCount, should.Equal(initialWUCount))
		assert.Loosely(t, listArtCount, should.Equal(initialParentArtCount))
		assert.Loosely(t, httpFetchCount, should.Equal(initialHTTPCount))

		// Call on sister child sharing same parent should reuse cached parent artifacts and parsed XML reason
		err3, dErr3 := DiscoverWorkUnitError(ctx, fakeClient, server.Client(), "rootInvocations/inv1/workUnits/child_wu2")
		assert.Loosely(t, dErr3, should.BeNil)
		assert.Loosely(t, err3, should.NotBeNil)
		assert.Loosely(t, listArtCount, should.Equal(initialParentArtCount)) // no extra list artifacts call on parent
		assert.Loosely(t, httpFetchCount, should.Equal(initialHTTPCount))    // no extra HTTP download
	})
}
