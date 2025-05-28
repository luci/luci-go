// Copyright 2019 The LUCI Authors.
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

// Package sink provides a server for aggregating test results and sending them
// to the ResultDB backend.
package sink

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/lucictx"
	"go.chromium.org/luci/server/middleware"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/resultdb/internal/schemes"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"
)

const (
	// AuthTokenKey is the key of the HTTP request header where the auth token should
	// be specified. Note that the an HTTP request must have the auth token in the header
	// with the following format.
	// Authorization: ResultSink <auth_token>
	AuthTokenKey = "Authorization"
	// AuthTokenPrefix is embedded into the value of the Authorization HTTP request header,
	// where the auth_token must be present. For the details about the value format of
	// the Authoization HTTP request header, find the description of `AuthTokenKey`.
	AuthTokenPrefix = "ResultSink"

	// maxWarmUpDuration is how long warmUp may wait until the Server is ready.
	//
	// Note that that retry.Default stops the iteration ~3m24s after the first try.
	// Any value greater than 3m24s won't make sense.
	maxWarmUpDuration = 30 * time.Second

	// DefaultArtChannelMaxLeases is the default value of ServerConfig.ArtChannelMaxLeases
	DefaultArtChannelMaxLeases = 16

	// DefaultTestResultChannelMaxLeases is the default value of ServerConfig.TestResultChannelMaxLeases
	DefaultTestResultChannelMaxLeases = 4
)

// ErrCloseBeforeStart is returned by Close(), when it was invoked before the server
// started.
var ErrCloseBeforeStart error = errors.New("the server is not started yet")

// ServerConfig defines the parameters of the server.
type ServerConfig struct {
	// Recorder is the gRPC client to the Recorder service exposed by ResultDB.
	Recorder pb.RecorderClient

	// ArtifactStreamClient is an HTTP client to be used for streaming artifacts larger than
	// MaxBatchableArtifactSize.
	ArtifactStreamClient *http.Client

	// ArtifactStreamHost is the hostname of an ResultDB service instance to which
	// artifacts are streamed.
	ArtifactStreamHost string

	// AuthToken is a secret token to expect from clients. If it is "" then it
	// will be randomly generated in a secure way.
	AuthToken string
	// Address is the HTTP address to listen on.
	//
	// If empty, the server will use "localhost" with a random port available at
	// the time of Server.Start call, and the generated address can be found at
	// Server.Config().Address.
	Address string

	// Invocation is the name of the invocation that test results should append to.
	Invocation string

	// invocationID is a cached copy of the ID extracted from `Invocation`.
	invocationID string

	// UpdateToken is the token that allows writes to Invocation.
	UpdateToken string

	// TestIDPrefix will be prepended to the test_id of each TestResult.
	// Setting this forces use of legacy (unstructured string) test IDs in uploads
	// and requires test harnesses support the same.
	// Note: even if this is unset, if ModuleName is not set, we default to uploading
	// legacy test IDs to preserve compatibility.
	// Must not be set in conjunction with ModuleName.
	TestIDPrefix string

	// The module name associated with uploaded test results.
	// Setting this enables use of structured test IDs in uploads and requires test
	// harness to support the same.
	// Must not be set in conjunction with TestIDPrefix.
	ModuleName string

	// The test scheme, as retrieved from the luci.resultdb.v1.Schema service.
	// Must be set. If ModuleName is set, a scheme other than 'legacy' is expected.
	// Otherwise, the scheme for 'legacy' is expected.
	// Refer to go/resultdb-schemes for information about test schemes.
	ModuleScheme *schemes.Scheme

	// The module variant.
	// This also sets the module variant for test results uploaded with legacy-form IDs.
	Variant *pb.Variant

	// Specifies the test_id prefix that will be combined with the (legacy) test_id
	// provided to ReportTestResults to set the test_metadata.previous_test_id field.
	//
	// Set to nil to disable this functionality. Setting this to an empty string
	// uploads the (legacy) test_Id to test_metadata.previous_test_id without modification.
	PreviousTestIDPrefix *string

	// TestLocationBase will be prepended to the Location.FileName of each TestResult.
	TestLocationBase string

	// ArtChannelMaxLeases specifies that the max lease of the Artifact upload channel.
	ArtChannelMaxLeases uint

	// TestResultChannelMaxLeases specifies that the max lease of the TestResult upload channel.
	TestResultChannelMaxLeases uint

	// BaseTags will be added to each TestResult in addition to the original tags that
	// the tests were reported with.
	BaseTags []*pb.StringPair

	// CoerceNegativeDuration specifies whether ReportTestResults() should coerece
	// the negative duration of a test result.
	//
	// If true, the API will coerce negative durations to 0.
	// If false, the API will return an error for negative durations.
	CoerceNegativeDuration bool

	// LocationTags is a map from a directory to the tags of this directory.
	// For each test with test location, look for the location's directory (or
	// ancestor directory) in the map and append the directory's tags to the
	// test results' tags.
	LocationTags *sinkpb.LocationTags

	// MaxBatchableArtifactSize is the maximum size of an artifact that can be uploaded
	// in a batch.
	//
	// Artifacts smaller or equal to this size will be uploaded in a batch, whereas
	// greater artifacts will be uploaded in a stream manner.
	// Must be < 10MiB, and NewServer panics, otherwise.
	MaxBatchableArtifactSize int64

	// ExonerateUnexpectedPass is a flag to control if an unexpected pass should
	// be exonerated.
	ExonerateUnexpectedPass bool

	// ShortenIDs controls whether test IDs should be automatically truncated by
	// result sink at upload time.
	ShortenIDs bool
}

// Validate validates all the config fields.
func (c *ServerConfig) Validate() error {
	if c.Recorder == nil {
		return errors.New("Recorder: unspecified")
	}
	if c.ArtifactStreamClient == nil {
		return errors.New("ArtifactStreamClient: unspecified")
	}
	if err := pbutil.ValidateStringPairs(c.BaseTags); err != nil {
		return errors.Fmt("BaseTags: %w", err)
	}
	if err := pbutil.ValidateVariant(c.Variant); err != nil {
		return errors.Fmt("Variant: %w", err)
	}
	if err := pbutil.ValidateInvocationName(c.Invocation); err != nil {
		return errors.Fmt("Invocation: %w", err)
	}
	if c.UpdateToken == "" {
		return errors.New("UpdateToken: unspecified")
	}
	if c.TestLocationBase != "" {
		if err := pbutil.ValidateFilePath(c.TestLocationBase); err != nil {
			return errors.Fmt("TestLocationBase: %w", err)
		}
	}
	if c.MaxBatchableArtifactSize > 10*1024*1024 {
		return errors.Fmt("MaxBatchableArtifactSize: %d is greater than 10MiB", c.MaxBatchableArtifactSize)
	}
	if c.ModuleScheme == nil {
		// Having the module name, scheme and variant locked-in upfront makes
		// it easy for us to start uploading work unit-level errors to each
		// module in future, based on the resultsink status.
		return errors.New("ModuleScheme: unspecified")
	}
	if c.ModuleName != "" {
		if err := pbutil.ValidateModuleName(c.ModuleName); err != nil {
			return errors.Fmt("ModuleName: %w", err)
		}
		if c.ModuleName == pbutil.LegacyModuleName {
			return errors.Fmt("ModuleName: cannot be %q", pbutil.LegacyModuleName)
		}
		if c.ModuleScheme.ID == pbutil.LegacySchemeID {
			return errors.New("ModuleScheme: may not be 'legacy' if ModuleName is set")
		}
		if c.TestIDPrefix != "" {
			return errors.New("TestIDPrefix: may not be set if ModuleName is set")
		}
	} else {
		if c.ModuleScheme.ID != pbutil.LegacySchemeID {
			return errors.New("ModuleScheme: should be 'legacy' if ModuleName is not set")
		}
	}
	if c.PreviousTestIDPrefix != nil {
		if pbutil.IsStructuredTestID(*c.PreviousTestIDPrefix) {
			return errors.New("PreviousTestIDPrefix: must not start with prefix of structured test IDs")
		}
		if err := pbutil.ValidateTestID(*c.PreviousTestIDPrefix); err != nil {
			return errors.Fmt("PreviousTestIDPrefix: %w", err)
		}
	}
	// For clients still using legacy test IDs, deprecated.
	if c.TestIDPrefix != "" {
		if pbutil.IsStructuredTestID(c.TestIDPrefix) {
			return errors.New("TestIDPrefix: must not start with prefix of structured test IDs")
		}
		if err := pbutil.ValidateTestID(c.TestIDPrefix); err != nil {
			return errors.Fmt("TestIDPrefix: %w", err)
		}
	}
	return nil
}

// Server contains state relevant to the server itself.
// It should always be created by a call to NewServer.
// After a call to Serve(), Server will accept connections on its Port and
// gather test results to send to its Recorder.
type Server struct {
	cfg     ServerConfig
	doneC   chan struct{}
	httpSrv http.Server

	// 1 indicates that the server is starting or has started. 0, otherwise.
	started int32

	mu  sync.Mutex // protects err
	err error
}

// NewServer creates a Server value and populates optional values with defaults.
func NewServer(ctx context.Context, cfg ServerConfig) (*Server, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Fmt("invalid ServerConfig: %w", err)
	}

	if cfg.AuthToken == "" {
		tk, err := genAuthToken(ctx)
		if err != nil {
			return nil, errors.Fmt("ServerConfig: failed to generate AuthToken: %w", err)
		}
		cfg.AuthToken = tk
	}
	if cfg.ArtChannelMaxLeases == 0 {
		cfg.ArtChannelMaxLeases = DefaultArtChannelMaxLeases
	}
	if cfg.TestResultChannelMaxLeases == 0 {
		cfg.TestResultChannelMaxLeases = DefaultTestResultChannelMaxLeases
	}
	if cfg.MaxBatchableArtifactSize == 0 {
		cfg.MaxBatchableArtifactSize = 2 * 1024 * 1024
	}

	// extract the invocation ID from cfg.Invocation so that other modules don't need to
	// parse the name to extract ID repeatedly.
	cfg.invocationID, _ = pbutil.ParseInvocationName(cfg.Invocation)

	s := &Server{
		cfg:   cfg,
		doneC: make(chan struct{}),
	}
	return s, nil
}

// Done returns a channel that is closed when the server terminated and finished processing
// all the ongoing requests.
func (s *Server) Done() <-chan struct{} {
	return s.doneC
}

// Config retrieves the ServerConfig of a previously created Server.
//
// Use this to retrieve the resolved values of unset optional fields in the
// original ServerConfig.
func (s *Server) Config() ServerConfig {
	return s.cfg
}

// Err returns a server error, explaining the reason of the sink server closed.
//
// If Done is not yet closed, Err returns nil.
// If Done is closed, Err returns a non-nil error explaining why.
func (s *Server) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Run starts a server and runs callback in a context where the server is running.
//
// The context passed to callback will be cancelled if the server has stopped due to
// critical errors or Close being invoked. The context also has the server's information
// exported into it. If callback finishes, Run will stop the server and return the error
// callback returned.
func Run(ctx context.Context, cfg ServerConfig, callback func(context.Context, ServerConfig) error) (err error) {
	s, err := NewServer(ctx, cfg)
	if err != nil {
		return err
	}
	if err := s.Start(ctx); err != nil {
		return err
	}
	defer func() {
		// Run returns nil IFF both of the callback and Shutdown returned nil.
		sErr := s.Shutdown(ctx)
		if err == nil {
			err = sErr
		}
	}()

	// ensure that the server is ready for serving traffic before executing the callback.
	logging.Infof(ctx, "SinkServer: warm-up started")
	if err = s.warmUp(ctx); err != nil {
		logging.Errorf(ctx, "SinkServer: warm-up failed: %s", err)
		return errors.Fmt("warm-up: %w", err)
	}
	logging.Infof(ctx, "SinkServer: warm-up ended")

	// It's necessary to create a new context with a new variable. If param `ctx` was
	// re-used, the new context would be passed to Shutdown in the deferred function after
	// being cancelled.
	cbCtx, cancel := context.WithCancel(s.Export(ctx))
	defer cancel()
	go func() {
		select {
		case <-s.Done():
			// cancel the callback context if the server terminates before callback
			// finishes.
			cancel()
		case <-ctx.Done():
		}
	}()
	return callback(cbCtx, cfg)
}

// Start runs the server.
//
// On success, Start will return nil, and a subsequent error can be obtained from Err.
func (s *Server) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&s.started, 0, 1) {
		return errors.New("cannot call Start twice")
	}

	// create an HTTP server with a pRPC service.
	routes := router.New()
	routes.Use(router.NewMiddlewareChain(middleware.WithPanicCatcher))
	s.httpSrv.Handler = routes
	s.httpSrv.BaseContext = func(net.Listener) context.Context { return ctx }
	ss, err := newSinkServer(ctx, s.cfg)
	if err != nil {
		return err
	}
	prpc := &prpc.Server{
		// Increase the limit to 1 GiB, since the default 64 MiB is too small.
		MaxRequestSize: 1024 * 1024 * 1024,
	}
	prpc.InstallHandlers(routes, nil)
	sinkpb.RegisterSinkServer(prpc, ss)

	addr := s.cfg.Address
	if addr == "" {
		// If the config is missing the address, choose a random available port.
		addr = "localhost:0"
	}

	l, err := net.Listen("tcp", addr)
	if err != nil {
		s.mu.Lock()
		s.err = err
		s.mu.Unlock()
		return err
	}
	if s.cfg.Address == "" {
		s.cfg.Address = fmt.Sprint("localhost:", l.Addr().(*net.TCPAddr).Port)
	}

	go func() {
		defer func() {
			// close SinkServer to complete all the outgoing requests before closing doneC
			// to send a signal.
			closeSinkServer(ctx, ss)
			close(s.doneC)
		}()

		logging.Infof(ctx, "SinkServer: starting HTTP server...")
		// No need to close the listener, because http.Serve always closes it on return.
		err = s.httpSrv.Serve(l)
		logging.Infof(ctx, "SinkServer: HTTP server stopped with %q", err)

		// if the reason of the server stopped was due to s.Close or s.Shutdown invoked,
		// then s.Err should return nil instead.
		if err == http.ErrServerClosed {
			err = nil
		}

		s.mu.Lock()
		defer s.mu.Unlock()
		s.err = err
	}()
	return nil
}

// Shutdown gracefully shuts down the server without interrupting any ongoing requests.
//
// Shutdown works by first closing the listener for incoming requests, and then waiting for
// all the ongoing requests to be processed and pending results to be uploaded. If
// the provided context expires before the shutdown is complete, Shutdown returns
// the context's error.
func (s *Server) Shutdown(ctx context.Context) (err error) {
	logging.Infof(ctx, "SinkServer: shutdown started")
	defer func() {
		if err == nil {
			logging.Infof(ctx, "SinkServer: shutdown completed successfully")
		} else {
			logging.Errorf(ctx, "SinkServer: shutdown failed: %s", err)
		}
	}()

	if atomic.LoadInt32(&s.started) == 0 {
		// hasn't been started
		return ErrCloseBeforeStart
	}
	if err = s.httpSrv.Shutdown(ctx); err != nil {
		return
	}

	select {
	case <-s.Done():
		// Shutdown always returns nil as long as the server was closed and drained.
	case <-ctx.Done():
		err = ctx.Err()
	}
	return
}

// Close stops the server.
//
// Server processes sinkRequests asynchronously, and Close doesn't guarantee completion
// of all the ongoing requests. After Close returns, wait for Done to be closed to ensure
// that all the pending results have been uploaded.
//
// It's recommended to use Shutdown() instead of Close with Done.
func (s *Server) Close(ctx context.Context) (err error) {
	logging.Infof(ctx, "SinkServer: close started")
	defer logging.Infof(ctx, "SinkServer: close completed with %s", err)

	if atomic.LoadInt32(&s.started) == 0 {
		// hasn't been started
		err = ErrCloseBeforeStart
		return
	}
	return s.httpSrv.Close()
}

// Export exports lucictx.ResultSink derived from the server configuration into
// the context.
func (s *Server) Export(ctx context.Context) context.Context {
	return lucictx.SetResultSink(ctx, &lucictx.ResultSink{
		Address:   s.cfg.Address,
		AuthToken: s.cfg.AuthToken,
	})
}

// genAuthToken generates and returns a random auth token.
func genAuthToken(ctx context.Context) (string, error) {
	buf := make([]byte, 32)
	if _, err := cryptorand.Read(ctx, buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// warmUp continuously sends an empty test result to the SinkServer until it receives
// a succeesful response.
//
// This function is used to ensure that the SinkServer is ready for serving traffic.
// It returns to the caller, if the context times out, it reached the maximum allowed
// retry count (10), or a non-error reponse was returned from the SinkServer.
func (s *Server) warmUp(ctx context.Context) error {
	sinkClient := sinkpb.NewSinkPRPCClient(&prpc.Client{
		Host:    s.cfg.Address,
		Options: &prpc.Options{Insecure: true},
	})
	ctx = metadata.AppendToOutgoingContext(ctx, AuthTokenKey, authTokenValue(s.cfg.AuthToken))
	ctx, cancel := context.WithTimeout(ctx, maxWarmUpDuration)
	defer cancel()
	return retry.Retry(ctx, retry.Default, func() error {
		_, err := sinkClient.ReportTestResults(ctx, &sinkpb.ReportTestResultsRequest{},
			// connection errors will happen until the server is ready for serving.
			prpc.ExpectedCode(codes.Internal))
		return err
	}, nil)
}
