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
	"encoding/hex"
	"fmt"
	"sync/atomic"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"

	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"
)

// sinkServer implements sinkpb.SinkServer.
type sinkServer struct {
	cfg           ServerConfig
	trChan        trChan
	resultIDBase  string
	resultCounter uint32
}

func newSinkServer(ctx context.Context, cfg ServerConfig) (sinkpb.SinkServer, error) {
	// random bytes to generate a ResultID when ResultID unspecified in
	// a TestResult.
	bytes := make([]byte, 4)
	if _, err := mathrand.Read(ctx, bytes); err != nil {
		return nil, err
	}
	ss := &sinkServer{
		cfg:          cfg,
		trChan:       trChan{cfg: cfg},
		resultIDBase: hex.EncodeToString(bytes),
	}
	if err := ss.trChan.init(ctx); err != nil {
		return nil, err
	}

	return &sinkpb.DecoratedSink{
		Service: ss,
		Prelude: authTokenPrelude(cfg.AuthToken),
	}, nil
}

// closeSinkServer closes the dispatcher channels and blocks until they are fully drained,
// or the context is cancelled.
func closeSinkServer(ctx context.Context, s sinkpb.SinkServer) {
	ss := s.(*sinkpb.DecoratedSink).Service.(*sinkServer)
	ss.trChan.closeAndDrain(ctx)
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
		tr.TestId = s.cfg.TestIDPrefix + tr.GetTestId()

		// assign a random, unique ID if resultID omitted.
		if tr.ResultId == "" {
			tr.ResultId = fmt.Sprintf("%s-%.5d", s.resultIDBase, atomic.AddUint32(&s.resultCounter, 1))
		}

		if err := validateTestResult(now, tr); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "bad request: %s", err)
		}
	}
	s.trChan.reportTestResults(in.TestResults)

	// TODO(1017288) - set `TestResultNames` in the response
	return &sinkpb.ReportTestResultsResponse{}, nil
}
