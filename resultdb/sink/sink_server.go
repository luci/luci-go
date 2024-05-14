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
	"path"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"
)

var zeroDuration = durationpb.New(0)

// sinkServer implements sinkpb.SinkServer.
type sinkServer struct {
	cfg           ServerConfig
	ac            *artifactChannel
	tc            *testResultChannel
	ec            *unexpectedPassChannel
	resultIDBase  string
	resultCounter uint32

	// A set of invocation-level artifact IDs that have been uploaded.
	// If an artifact is uploaded again, server should reject the request with
	// error AlreadyExists.
	invocationArtifactIDs stringset.Set
	mu                    sync.Mutex
}

func newSinkServer(ctx context.Context, cfg ServerConfig) (sinkpb.SinkServer, error) {
	// random bytes to generate a ResultID when ResultID unspecified in
	// a TestResult.
	bytes := make([]byte, 4)
	if _, err := mathrand.Read(ctx, bytes); err != nil {
		return nil, err
	}

	ctx = metadata.AppendToOutgoingContext(ctx, pb.UpdateTokenMetadataKey, cfg.UpdateToken)
	ss := &sinkServer{
		cfg:                   cfg,
		ac:                    newArtifactChannel(ctx, &cfg),
		tc:                    newTestResultChannel(ctx, &cfg),
		resultIDBase:          hex.EncodeToString(bytes),
		invocationArtifactIDs: stringset.New(0),
	}

	if cfg.ExonerateUnexpectedPass {
		ss.ec = newTestExonerationChannel(ctx, &cfg)
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

	logging.Infof(ctx, "SinkServer: draining TestResult channel started")
	ss.tc.closeAndDrain(ctx)
	logging.Infof(ctx, "SinkServer: draining TestResult channel ended")

	logging.Infof(ctx, "SinkServer: draining Artifact channel started")
	ss.ac.closeAndDrain(ctx)
	logging.Infof(ctx, "SinkServer: draining Artifact channel ended")

	if ss.ec != nil {
		logging.Infof(ctx, "SinkServer: draining TestExoneration channel started")
		ss.ec.closeAndDrain(ctx)
		logging.Infof(ctx, "SinkServer: draining TestExoneration channel ended")
	}
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

	// create a slice with a rough estimate.
	uts := make([]*uploadTask, 0, len(in.TestResults)*4)
	// Unexpected passed test results that need to be exonerated.
	trsForExo := make([]*sinkpb.TestResult, 0, len(in.TestResults))
	for _, tr := range in.TestResults {
		tr.TestId = s.cfg.TestIDPrefix + tr.GetTestId()

		// assign a random, unique ID if resultID omitted.
		if tr.ResultId == "" {
			tr.ResultId = fmt.Sprintf("%s-%.5d", s.resultIDBase, atomic.AddUint32(&s.resultCounter, 1))
		}

		if s.cfg.TestLocationBase != "" {
			locFn := tr.GetTestMetadata().GetLocation().GetFileName()
			// path.Join converts the leading double slashes to a single slash.
			// Add a slash to keep the double slashes.
			if locFn != "" && !strings.HasPrefix(locFn, "//") {
				tr.TestMetadata.Location.FileName = "/" + path.Join(s.cfg.TestLocationBase, locFn)
			}
		}
		// The system-clock of GCE machines may get updated by ntp while a test is running.
		// It can possibly cause a negative duration produced, because most test harnesses
		// use system-clock to calculate the run time of a test. For more info, visit
		// crbug.com/1135892.
		if duration := tr.GetDuration(); duration != nil && s.cfg.CoerceNegativeDuration {
			// If a negative duration was reported, remove the duration.
			if d := duration.AsDuration(); d < 0 {
				logging.Warningf(ctx, "TestResult(%s) has a negative duration(%s); coercing it to 0", tr.TestId, d)
				tr.Duration = zeroDuration
			}
		}

		if err := validateTestResult(now, tr); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "%s", err)
		}

		for id, a := range tr.GetArtifacts() {
			n := pbutil.TestResultArtifactName(s.cfg.invocationID, tr.TestId, tr.ResultId, id)
			t, err := newUploadTask(n, a, tr.Status)

			// newUploadTask can return an error if os.Stat() fails.
			if err != nil {
				// TODO(crbug.com/1124868) - once all test harnesses are fixed, return 4xx on
				// newUploadTask failures instead of dropping the artifact silently.
				logging.Warningf(ctx, "Dropping an artifact request; failed to create a new Uploadtask: %s",
					errors.Annotate(err, "artifact %q: %s", id, err).Err())
				continue
			}
			uts = append(uts, t)
		}

		if s.ec != nil && !tr.Expected && tr.Status == pb.TestStatus_PASS {
			trsForExo = append(trsForExo, tr)
		}
	}

	parallel.FanOutIn(func(work chan<- func() error) {
		work <- func() error {
			s.ac.schedule(uts...)
			return nil
		}
		work <- func() error {
			s.tc.schedule(in.TestResults...)
			return nil
		}
		if s.ec != nil {
			work <- func() error {
				s.ec.schedule(trsForExo...)
				return nil
			}
		}
	})

	// TODO(1017288) - set `TestResultNames` in the response
	return &sinkpb.ReportTestResultsResponse{}, nil
}

// ReportInvocationLevelArtifacts implement sinkpb.SinkServer.
func (s *sinkServer) ReportInvocationLevelArtifacts(ctx context.Context, in *sinkpb.ReportInvocationLevelArtifactsRequest) (*emptypb.Empty, error) {
	uts := make([]*uploadTask, 0, len(in.Artifacts))
	for id, a := range in.Artifacts {
		if err := validateArtifact(a); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "bad request for artifact %q: %s", id, err)
		}
		t, err := newUploadTask(pbutil.InvocationArtifactName(s.cfg.invocationID, id), a, pb.TestStatus_STATUS_UNSPECIFIED)
		// newUploadTask can return an error if os.Stat() fails.
		if err != nil {
			// TODO(crbug.com/1124868) - once all test harnesses are fixed, return 4xx on
			// newUploadTask failures instead of dropping the artifact silently.
			logging.Warningf(ctx, "Dropping an artifact request; failed to create a new Uploadtask: %s",
				errors.Annotate(err, "artifact %q: %s", id, err).Err())
			continue
		}
		s.mu.Lock()
		if added := s.invocationArtifactIDs.Add(id); !added {
			s.mu.Unlock()
			return nil, status.Errorf(codes.AlreadyExists, "artifact %q has already been uploaded", id)
		}
		s.mu.Unlock()
		uts = append(uts, t)
	}
	s.ac.schedule(uts...)

	return &emptypb.Empty{}, nil
}

// UpdateInvocation implement sinkpb.SinkServer
func (s *sinkServer) UpdateInvocation(ctx context.Context, sinkin *sinkpb.UpdateInvocationRequest) (*sinkpb.Invocation, error) {
	// We are running this method (but ignoring its result) to validate the
	// mask only refers to fields in the sinkpb.Invocation proto.
	if _, err := mask.FromFieldMask(sinkin.UpdateMask, sinkin.Invocation, false, true); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "update_mask: %s", err)
	}
	inv, err := s.cfg.Recorder.UpdateInvocation(ctx, &pb.UpdateInvocationRequest{
		Invocation: &pb.Invocation{
			Name:               pbutil.InvocationName(s.cfg.invocationID),
			ExtendedProperties: sinkin.Invocation.GetExtendedProperties(),
		},
		UpdateMask: sinkin.GetUpdateMask(),
	})
	if err != nil {
		return nil, err
	}
	ret := &sinkpb.Invocation{
		ExtendedProperties: inv.GetExtendedProperties(),
	}
	return ret, nil
}
