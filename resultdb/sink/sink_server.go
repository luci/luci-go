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
	"net/http"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/resultdb/pbutil"
	rpcpb "go.chromium.org/luci/resultdb/proto/rpc/v1"
	sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"
	"go.chromium.org/luci/server/auth"
)

// sinkServer implements sinkpb.SinkServer.
type sinkServer struct {
	cfg     ServerConfig
	closeFn func()

	recCh    *dispatcher.Channel
	recorder rpcpb.RecorderClient
}

type artifactWorkload struct {
	name     string
	artifact *sinkpb.Artifact
}

func newSinkServer(ctx context.Context, cfg ServerConfig) (sinkpb.SinkServer, error) {
	ss := &sinkServer{cfg: cfg}
	if err := ss.initClients(ctx); err != nil {
		return nil, err
	}
	if err := ss.initChannels(ctx); err != nil {
		return nil, err
	}
	return &sinkpb.DecoratedSink{
		Service: ss,
		Prelude: authTokenPrelude(cfg.AuthToken),
	}, nil
}

func (s *sinkServer) initClients(ctx context.Context) error {
	transport, err := auth.GetRPCTransport(ctx, auth.AsSelf)
	if err != nil {
		return err
	}
	pc := &prpc.Client{C: &http.Client{Transport: transport}, Host: s.cfg.ResultDBHost}
	s.recorder = rpcpb.NewRecorderPRPCClient(pc)
	// TODO(1017288) - add gs client
	return nil
}

func (s *sinkServer) initChannels(ctx context.Context) error {
	recSendFn := func(b *buffer.Batch) error {
		var trs []*sinkpb.TestResult
		for _, d := range b.Data {
			tr, ok := d.(*sinkpb.TestResult)
			if !ok {
				logging.Errorf(ctx, "invalid message type for reportTestResults")
				continue
			}
			trs = append(trs, tr)
		}
		return reportTestResults(s.recorder, s.cfg.Invocation, trs)
	}
	rc, err := dispatcher.NewChannel(ctx, &s.cfg.RecorderChannelOpts, recSendFn)
	if err != nil {
		return err
	}
	s.recCh = &rc
	// TODO(1017288) - add gs channel
	return nil
}

// closeSinkServer closes the dispatcher channels and blocks until all the in-flight items
// are processed or the context is cancelled.
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

func reportTestResults(rec rpcpb.RecorderClient, inv string, trs []*sinkpb.TestResult) error {
	// TODO(1017288) - invoke Recorder.CreateTestResult()
	return nil
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
