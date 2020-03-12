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
	"sync"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	"go.chromium.org/luci/resultdb/pbutil"
	sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"
)

// sinkServer implements sinkpb.SinkServer.
type sinkServer struct {
	cfg     ServerConfig
	recC    *dispatcher.Channel
	gsC     *dispatcher.Channel
	closeFn func()
}

type artifactWorkload struct {
	name     string
	artifact *sinkpb.Artifact
}

type ctxSendFn func(context.Context, *buffer.Batch) error

func withCtx(ctx context.Context, f ctxSendFn) dispatcher.SendFn {
	return func(b *buffer.Batch) error {
		return f(ctx, b)
	}
}

func newSinkServer(ctx context.Context, cfg ServerConfig) (sinkpb.SinkServer, error) {
	rd, err := dispatcher.NewChannel(ctx, &cfg.RecorderChannelOpts, withCtx(ctx, reportTestResults))
	if err != nil {
		return nil, err
	}
	gd, err := dispatcher.NewChannel(ctx, &cfg.GsChannelOpts, withCtx(ctx, uploadArtifacts))
	if err != nil {
		rd.Close()
		return nil, err
	}

	return &sinkpb.DecoratedSink{
		Service: &sinkServer{
			cfg:     cfg,
			recC:    &rd,
			gsC:     &gd,
			closeFn: func() { closeAndDrainChannels(ctx, &rd, &gd) },
		},
		Prelude: authTokenPrelude(cfg.AuthToken),
	}, nil
}

func closeSinkServer(ss sinkpb.SinkServer) {
	dec := ss.(*sinkpb.DecoratedSink)
	dec.Service.(*sinkServer).closeFn()
}

func closeAndDrainChannels(ctx context.Context, chs ...*dispatcher.Channel) {
	var wg sync.WaitGroup
	wg.Add(len(chs))
	for _, ch := range chs {
		go func() {
			defer wg.Done()
			ch.CloseAndDrain(ctx)
		}()
	}
	wg.Wait()
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

func reportTestResults(ctx context.Context, b *buffer.Batch) error {
	for _, d := range b.Data {
		_, ok := d.(*sinkpb.TestResult)
		if !ok {
			logging.Errorf(ctx, "invalid message type for reportTestResults")
			return nil
		}
		// TODO(1017288) - invoke Recorder.CreateTestResult()
	}
	return nil
}

func uploadArtifacts(ctx context.Context, b *buffer.Batch) error {
	for _, d := range b.Data {
		_, ok := d.(*artifactWorkload)
		if !ok {
			logging.Errorf(ctx, "invalid message type for uploadArtifacts")
			return nil
		}
		// TODO(1017288) - invoke Google Storage APIs
	}
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

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, tr := range in.TestResults {
			for name, art := range tr.InputArtifacts {
				s.gsC.C <- artifactWorkload{name, art}
			}
			for name, art := range tr.OutputArtifacts {
				s.gsC.C <- artifactWorkload{name, art}
			}
		}
	}()
	for _, tr := range in.TestResults {
		s.recC.C <- tr
	}
	wg.Wait()

	// TODO(1017288) - set `TestResultNames` in the response
	return &sinkpb.ReportTestResultsResponse{}, nil
}
