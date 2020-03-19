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
	"os"
	"sort"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/google/uuid"
	"golang.org/x/time/rate"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
	sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"
)

// recorderDispatcherOpts returns the channel options for Recorder.
func recorderDispatcherOpts() *dispatcher.Options {
	return &dispatcher.Options{
		QPSLimit: rate.NewLimiter(1, 1),
		Buffer: buffer.Options{
			BatchSize:     400,
			MaxLeases:     4,
			BatchDuration: 10 * time.Second,
			FullBehavior:  &buffer.BlockNewItems{MaxItems: 2000},
		},
	}
}

// sinkServer implements sinkpb.SinkServer.
type sinkServer struct {
	cfg ServerConfig

	recCh    *dispatcher.Channel
	recorder pb.RecorderClient
}

func newSinkServer(ctx context.Context, cfg ServerConfig) (sinkpb.SinkServer, error) {
	ss := &sinkServer{cfg: cfg}
	if err := ss.initChannels(ctx); err != nil {
		return nil, err
	}
	return &sinkpb.DecoratedSink{
		Service: ss,
		Prelude: authTokenPrelude(cfg.AuthToken),
	}, nil
}

func (s *sinkServer) initChannels(ctx context.Context) error {
	// install a dispatcher channel for Recorder
	rc, err := dispatcher.NewChannel(ctx, recorderDispatcherOpts(), func(b *buffer.Batch) error {
		req := prepareReportTestResultsRequest(ctx, s.cfg.Invocation, b)
		_, err := s.cfg.Recorder.BatchCreateTestResults(ctx, req)
		return err
	})
	if err != nil {
		return err
	}
	s.recCh = &rc

	// TODO(crbug.com/1017288) - add gs channel
	return nil
}

func prepareReportTestResultsRequest(ctx context.Context, inv string, b *buffer.Batch) *pb.BatchCreateTestResultsRequest {
	// retried batch?
	if b.Meta != nil {
		return b.Meta.(*pb.BatchCreateTestResultsRequest)
	}
	req := &pb.BatchCreateTestResultsRequest{
		Invocation: inv,
		// a random UUID
		RequestId: uuid.New().String(),
	}
	for _, d := range b.Data {
		tr := d.(*sinkpb.TestResult)
		req.Requests = append(req.Requests, &pb.CreateTestResultRequest{
			TestResult: &pb.TestResult{
				TestId:          tr.GetTestId(),
				ResultId:        tr.GetResultId(),
				Variant:         tr.GetVariant(),
				Expected:        tr.GetExpected(),
				SummaryHtml:     tr.GetSummaryHtml(),
				StartTime:       tr.GetStartTime(),
				Duration:        tr.GetDuration(),
				Tags:            tr.GetTags(),
				InputArtifacts:  sinkArtsToRpcArts(ctx, tr.GetInputArtifacts()),
				OutputArtifacts: sinkArtsToRpcArts(ctx, tr.GetOutputArtifacts()),
			},
		})
	}
	b.Meta = req
	return req
}

func sinkArtsToRpcArts(ctx context.Context, sArts map[string]*sinkpb.Artifact) (rArts []*pb.Artifact) {
	for name, sart := range sArts {
		var size int64 = -1
		switch {
		case sart.GetFilePath() != "":
			if info, err := os.Stat(sart.GetFilePath()); err == nil {
				size = info.Size()
			} else {
				logging.Errorf(ctx, "artifact %q: %q - %s", name, sart.GetFilePath(), err)
			}
		case sart.GetContents() != nil:
			size = int64(len(sart.GetContents()))
		default:
			// This should never be reached. pbutil.ValidateSinkArtifact() should
			// filter out invalid artifacts.
			panic(fmt.Sprintf("%s: neither file_path nor contents were given", name))
		}

		rArts = append(rArts, &pb.Artifact{
			Name: name,
			// TODO(ddoman): set fetch_url and fetch_url_expiration
			ContentType: sart.GetContentType(),
			Size:        size,
		})
	}
	sort.Slice(rArts, func(i, j int) bool {
		return rArts[i].Name < rArts[j].Name
	})
	return
}

// closeSinkServer closes the dispatcher channels and blocks until they are fully drained,
// or the context is cancelled.
func closeSinkServer(ctx context.Context, s sinkpb.SinkServer) {
	ss := s.(*sinkpb.DecoratedSink).Service.(*sinkServer)
	ss.recCh.CloseAndDrain(ctx)
}

// authTokenValue returns the value of the Authorization HTTP header that all requests must
// have.
func authTokenValue(authToken string) string {
	return fmt.Sprintf("%s %s", AuthTokenPrefix, authToken)
}

// authTokenValidator is a factory function generating a pRPC prelude that validates
// a given HTTP request with the auth key.
func authTokenPrelude(authToken string) func(context.Context, string, proto.Message) (context.Context, error) {
	expected := authTokenValue(authToken)
	missingKeyErr := status.Errorf(codes.Unauthenticated, "Authorization header is missing")

	return func(ctx context.Context, _ string, _ proto.Message) (context.Context, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, missingKeyErr
		}
		tks := md.Get(AuthTokenKey)
		if len(tks) == 0 {
			return nil, missingKeyErr
		}
		for _, tk := range tks {
			if tk == expected {
				return ctx, nil
			}
		}
		return nil, status.Errorf(codes.PermissionDenied, "no valid auth_token found")
	}
}

// ReportTestResults implement sinkpb.SinkServer.
func (s *sinkServer) ReportTestResults(ctx context.Context, in *sinkpb.ReportTestResultsRequest) (*sinkpb.ReportTestResultsResponse, error) {
	now := clock.Now(ctx).UTC()
	for _, tr := range in.TestResults {
		if err := pbutil.ValidateSinkTestResult(now, tr); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "bad request: %s", err)
		}
	}
	for _, tr := range in.TestResults {
		s.recCh.C <- tr
	}
	// TODO(1017288) - set `TestResultNames` in the response
	return &sinkpb.ReportTestResultsResponse{}, nil
}
