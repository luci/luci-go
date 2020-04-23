// Copyright 2020 The LUCI Authors.
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

package sink

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/ptypes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/grpc/prpc"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
	sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func installTestListener(srv *Server) (string, func() error) {
	l, err := net.Listen("tcp", "localhost:0")
	So(err, ShouldBeNil)
	srv.testListener = l

	// return the serving address
	return fmt.Sprint("localhost:", l.Addr().(*net.TCPAddr).Port), l.Close
}

func reportTestResults(ctx context.Context, host, authToken string, in *sinkpb.ReportTestResultsRequest) (*sinkpb.ReportTestResultsResponse, error) {
	sinkClient := sinkpb.NewSinkPRPCClient(&prpc.Client{
		Host:    host,
		Options: &prpc.Options{Insecure: true},
	})
	// install the auth token into the context, if present
	if authToken != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, AuthTokenKey, authTokenValue(authToken))
	}
	return sinkClient.ReportTestResults(ctx, in)
}

func testServerConfig(ctl *gomock.Controller, addr, tk string) ServerConfig {
	return ServerConfig{
		Address:   addr,
		AuthToken: tk,
		Recorder:  pb.NewMockRecorderClient(ctl),
	}
}

func testArtifactWithFile(writer func(f *os.File)) *sinkpb.Artifact {
	f, err := ioutil.TempFile("", "test-artifact")
	So(err, ShouldBeNil)
	defer f.Close()
	writer(f)

	return &sinkpb.Artifact{
		Body:        &sinkpb.Artifact_FilePath{FilePath: f.Name()},
		ContentType: "text/plain",
	}
}

func testArtifactWithContents(contents []byte) *sinkpb.Artifact {
	return &sinkpb.Artifact{
		Body:        &sinkpb.Artifact_Contents{contents},
		ContentType: "text/plain",
	}
}

// validTestResult returns a valid sinkpb.TestResult sample message.
func validTestResult() (*sinkpb.TestResult, func()) {
	now := testclock.TestRecentTimeUTC
	st, _ := ptypes.TimestampProto(now.Add(-2 * time.Minute))
	artf := testArtifactWithFile(func(f *os.File) {
		_, err := f.WriteString("a sample artifact")
		So(err, ShouldBeNil)
	})
	cleanup := func() { os.Remove(artf.GetFilePath()) }

	return &sinkpb.TestResult{
		TestId:      "this is testID",
		ResultId:    "result_id1",
		Variant:     pbutil.Variant("a", "b"),
		Expected:    true,
		Status:      sinkpb.TestStatus_PASS,
		SummaryHtml: "HTML summary",
		StartTime:   st,
		Duration:    ptypes.DurationProto(time.Minute),
		Tags:        pbutil.StringPairs("k1", "v1"),
		Artifacts: map[string]*sinkpb.Artifact{
			"art1": artf,
		},
	}, cleanup
}

type invMatcher string

func invEq(inv string) gomock.Matcher {
	return invMatcher(inv)
}
func (m invMatcher) Matches(x interface{}) bool {
	req, ok := x.(*pb.BatchCreateTestResultsRequest)
	if !ok {
		return false
	}
	return req.Invocation == string(m)
}

func (m invMatcher) String() string {
	return fmt.Sprint("has Invocation ", string(m))
}
