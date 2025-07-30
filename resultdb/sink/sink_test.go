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

package sink

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/lucictx"

	"go.chromium.org/luci/resultdb/pbutil"
	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"
)

func TestNewServer(t *testing.T) {
	t.Parallel()

	ftt.Run("NewServer", t, func(t *ftt.Test) {
		ctx := context.Background()
		cfg := testServerConfig(":42", "my_token")

		t.Run("succeeds", func(t *ftt.Test) {
			srv, err := NewServer(ctx, cfg)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, srv, should.NotBeNil)
		})
		t.Run("generates a random auth token, if missing", func(t *ftt.Test) {
			cfg.AuthToken = ""
			srv, err := NewServer(ctx, cfg)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, srv.cfg.AuthToken, should.NotEqual(""))
		})
		t.Run("uses the default max leases, if missing or 0", func(t *ftt.Test) {
			srv, err := NewServer(ctx, cfg)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, srv.cfg.ArtChannelMaxLeases, should.Equal[uint](DefaultArtChannelMaxLeases))
			assert.Loosely(t, srv.cfg.TestResultChannelMaxLeases, should.Equal[uint](DefaultTestResultChannelMaxLeases))
		})
		t.Run("use the custom max leases, if specified", func(t *ftt.Test) {
			cfg.ArtChannelMaxLeases, cfg.TestResultChannelMaxLeases = 123, 456
			srv, err := NewServer(ctx, cfg)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, srv.cfg.ArtChannelMaxLeases, should.Equal[uint](123))
			assert.Loosely(t, srv.cfg.TestResultChannelMaxLeases, should.Equal[uint](456))
			testServerConfig("", "my_token")
		})
		t.Run("with TestLocationBase", func(t *ftt.Test) {
			// empty
			cfg.TestLocationBase = ""
			_, err := NewServer(ctx, cfg)
			assert.Loosely(t, err, should.BeNil)

			// valid
			cfg.TestLocationBase = "//base"
			_, err = NewServer(ctx, cfg)
			assert.Loosely(t, err, should.BeNil)

			// invalid - not starting with double slahes
			cfg.TestLocationBase = "base"
			_, err = NewServer(ctx, cfg)
			assert.Loosely(t, err, should.ErrLike("TestLocationBase: doesn't start with //"))
		})
		t.Run("with BaseTags", func(t *ftt.Test) {
			// empty
			cfg.BaseTags = nil
			_, err := NewServer(ctx, cfg)
			assert.Loosely(t, err, should.BeNil)

			// valid - unique keys
			cfg.BaseTags = pbutil.StringPairs("k1", "v1", "k2", "v2")
			_, err = NewServer(ctx, cfg)
			assert.Loosely(t, err, should.BeNil)

			// valid - duplicate keys
			cfg.BaseTags = pbutil.StringPairs("k1", "v1", "k1", "v2")
			_, err = NewServer(ctx, cfg)
			assert.Loosely(t, err, should.BeNil)

			// valid - empty value
			cfg.BaseTags = pbutil.StringPairs("k1", "")
			_, err = NewServer(ctx, cfg)
			assert.Loosely(t, err, should.BeNil)

			// invalid - empty key
			cfg.BaseTags = pbutil.StringPairs("", "v1")
			_, err = NewServer(ctx, cfg)
			assert.Loosely(t, err, should.ErrLike("key: unspecified"))
		})
		t.Run("with MaxBatchableArtifactSize", func(t *ftt.Test) {
			// default
			cfg.MaxBatchableArtifactSize = 0
			s, err := NewServer(ctx, cfg)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, s.cfg.MaxBatchableArtifactSize, should.NotEqual(0))

			// valid
			cfg.MaxBatchableArtifactSize = 512 * 1024
			_, err = NewServer(ctx, cfg)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfg.MaxBatchableArtifactSize, should.NotEqual(0))

			cfg.MaxBatchableArtifactSize = pbutil.MaxBatchRequestSize
			_, err = NewServer(ctx, cfg)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfg.MaxBatchableArtifactSize, should.NotEqual(0))

			// invalid - too big
			cfg.MaxBatchableArtifactSize = pbutil.MaxBatchRequestSize + 1
			_, err = NewServer(ctx, cfg)
			assert.Loosely(t, err, should.ErrLike("MaxBatchableArtifactSize: greater than maximum allowed value"))
		})
	})
}

func TestServer(t *testing.T) {
	t.Parallel()

	ftt.Run("Server", t, func(t *ftt.Test) {
		req := &sinkpb.ReportTestResultsRequest{}
		ctx := context.Background()

		srvCfg := testServerConfig("", "secret")
		srv, err := NewServer(ctx, srvCfg)
		assert.Loosely(t, err, should.BeNil)

		t.Run("Start assigns a random port, if missing cfg.Address", func(t *ftt.Test) {
			assert.Loosely(t, srv.Config().Address, should.BeEmpty)
			assert.Loosely(t, srv.Start(ctx), should.BeNil)
			assert.Loosely(t, srv.Config().Address, should.NotBeEmpty)
		})
		t.Run("Start fails", func(t *ftt.Test) {
			assert.Loosely(t, srv.Start(ctx), should.BeNil)

			t.Run("if called twice", func(t *ftt.Test) {
				assert.Loosely(t, srv.Start(ctx), should.ErrLike("cannot call Start twice"))
			})

			t.Run("after being closed", func(t *ftt.Test) {
				assert.Loosely(t, srv.Close(ctx), should.BeNil)
				assert.Loosely(t, srv.Start(ctx), should.ErrLike("cannot call Start twice"))
			})
		})

		t.Run("Close closes the HTTP server", func(t *ftt.Test) {
			assert.Loosely(t, srv.Start(ctx), should.BeNil)
			assert.Loosely(t, srv.Close(ctx), should.BeNil)

			_, err := reportTestResults(ctx, srv.Config().Address, "secret", req)
			// The error could be a connection error or write-error.
			// e.g.,
			// "No connection could be made", "connection refused", "write: broken pipe"
			//
			// The error messages could be different by OS, and this test simply checks
			// whether err != nil.
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("Close fails before Start being called", func(t *ftt.Test) {
			assert.Loosely(t, srv.Close(ctx), should.ErrLike(ErrCloseBeforeStart))
		})

		t.Run("Shutdown closes Done", func(t *ftt.Test) {
			isClosed := func() bool {
				select {
				case <-srv.Done():
					return true
				default:
					return false
				}
			}
			assert.Loosely(t, srv.Start(ctx), should.BeNil)

			// wait until the server is up.
			_, err := reportTestResults(ctx, srv.Config().Address, "secret", req)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, isClosed(), should.BeFalse)
			assert.Loosely(t, srv.Shutdown(ctx), should.BeNil)
			assert.Loosely(t, isClosed(), should.BeTrue)
		})

		t.Run("Run", func(t *ftt.Test) {
			handlerErr := make(chan error, 1)
			runErr := make(chan error)
			expected := errors.New("an error-1")

			t.Run("succeeds", func(t *ftt.Test) {
				// launch a go routine with Run
				go func() {
					runErr <- Run(ctx, srvCfg, func(ctx context.Context, cfg ServerConfig) error {
						return <-handlerErr
					})
				}()

				// finish the callback and verify that srv.Run returned what the callback
				// returned.
				handlerErr <- expected
				assert.Loosely(t, <-runErr, should.Equal(expected))
			})

			t.Run("serves requests", func(t *ftt.Test) {
				assert.Loosely(t, srv.Start(ctx), should.BeNil)

				t.Run("with 200 OK", func(t *ftt.Test) {
					res, err := reportTestResults(ctx, srv.Config().Address, "secret", req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res, should.NotBeNil)
				})

				t.Run("with 401 Unauthorized if the auth_token missing", func(t *ftt.Test) {
					_, err := reportTestResults(ctx, srv.Config().Address, "", req)
					assert.Loosely(t, status.Code(err), should.Equal(codes.Unauthenticated))
				})

				t.Run("with 403 Forbidden if auth_token mismatched", func(t *ftt.Test) {
					_, err := reportTestResults(ctx, srv.Config().Address, "not-a-secret", req)
					assert.Loosely(t, status.Code(err), should.Equal(codes.PermissionDenied))
				})
			})
		})
	})
}

func TestServerExport(t *testing.T) {
	t.Parallel()

	ftt.Run("Export returns the configured address and auth_token", t, func(t *ftt.Test) {
		ctx := context.Background()
		srv, err := NewServer(ctx, testServerConfig(":42", "hello"))
		assert.Loosely(t, err, should.BeNil)

		ctx = srv.Export(ctx)
		sink := lucictx.GetResultSink(ctx)
		assert.Loosely(t, sink, should.NotBeNil)
		assert.Loosely(t, sink, should.NotBeNil)
		assert.Loosely(t, sink.Address, should.Equal(":42"))
		assert.Loosely(t, sink.AuthToken, should.Equal("hello"))
	})
}
