// Copyright 2024 The LUCI Authors.
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

package servertest

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/lucictx"

	"go.chromium.org/luci/server"
	srvauth "go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/internal/testpb"
)

func TestEmptyServer(t *testing.T) {
	// t.Parallel() -- RunServer cannot run in parallel.
	srv, err := RunServer(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown()
}

func TestInitCallbackError(t *testing.T) {
	// t.Parallel() -- RunServer cannot run in parallel.
	boomErr := errors.New("BOOM")
	srv, err := RunServer(context.Background(), &Settings{
		Init: func(srv *server.Server) error {
			return boomErr
		},
	})
	assert.That(t, errors.Unwrap(err), should.Equal(boomErr))
	assert.Loosely(t, srv, should.BeNil)
}

func TestServerTestAndFakeAuthDb(t *testing.T) {
	// t.Parallel() -- RunServer cannot run in parallel.
	ctx := context.Background()

	srv, err := RunServer(ctx, &Settings{
		Options: &server.Options{
			AuthDBProvider: (&authtest.FakeDB{}).AsProvider(),
		},
	})

	assert.That(t, err, should.ErrLike(nil))
	assert.Loosely(t, srv, should.NotBeNil)

	srv.Shutdown()
}

func TestFakeRPCAuth(t *testing.T) {
	// t.Parallel() -- RunServer cannot run in parallel.
	testPb := &testPbServer{}
	srv, err := RunServer(context.Background(), &Settings{
		Options: &server.Options{
			OpenIDRPCAuthEnable: true,
		},
		Init: func(srv *server.Server) error {
			testpb.RegisterTestServer(srv, testPb)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown()

	clientCtx := lucictx.SetLocalAuth(context.Background(), srv.FakeClientRPCAuth())

	testClient := func(opts auth.Options) (testpb.TestClient, error) {
		tr, err := auth.NewAuthenticator(clientCtx, auth.SilentLogin, opts).Transport()
		if err != nil {
			return nil, err
		}
		return testpb.NewTestClient(&prpc.Client{
			C:    &http.Client{Transport: tr},
			Host: srv.HTTPAddr(),
			Options: &prpc.Options{
				Insecure: true,
			},
		}), nil
	}

	t.Run("OAuth token", func(t *testing.T) {
		testPb.calls = nil
		cl, err := testClient(auth.Options{
			Scopes: []string{"a", "b"},
		})
		assert.That(t, err, should.ErrLike(nil))
		_, err = cl.Unary(clientCtx, &testpb.Request{})
		assert.That(t, err, should.ErrLike(nil))
		assert.That(t, testPb.calls, should.Match([]fakeCall{
			{
				Caller: FakeClientIdentity,
				Kind:   "oauth",
				Scopes: []string{"a", "b"},
			},
		}))
	})

	t.Run("ID token", func(t *testing.T) {
		testPb.calls = nil
		cl, err := testClient(auth.Options{
			UseIDTokens: true,
			Audience:    "http://" + srv.HTTPAddr(),
		})
		assert.That(t, err, should.ErrLike(nil))
		_, err = cl.Unary(clientCtx, &testpb.Request{})
		assert.That(t, err, should.ErrLike(nil))
		assert.That(t, testPb.calls, should.Match([]fakeCall{
			{
				Caller: FakeClientIdentity,
				Kind:   "id",
				Aud:    "http://" + srv.HTTPAddr(),
			},
		}))
	})
}

type testPbServer struct {
	testpb.UnimplementedTestServer

	calls []fakeCall
}

type fakeCall struct {
	Caller identity.Identity
	Kind   string
	Scopes []string
	Aud    string
}

func (s *testPbServer) Unary(ctx context.Context, _ *testpb.Request) (*testpb.Response, error) {
	call := fakeCall{Caller: srvauth.CurrentIdentity(ctx)}
	if tok := GetFakeToken(ctx); tok != nil {
		call.Kind = tok.Kind
		call.Scopes = tok.Scopes
		call.Aud = tok.Aud
	}
	s.calls = append(s.calls, call)
	return &testpb.Response{}, nil
}
