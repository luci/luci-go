// Copyright 2023 The LUCI Authors.
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

package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"

	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/sidecar"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

var (
	testPerm0 = realms.RegisterPermission("fake.permission.0")
	testPerm1 = realms.RegisterPermission("fake.permission.1")
)

func TestAuthServer(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx := authtest.MockAuthConfig(context.Background())
		ctx = auth.ModifyConfig(ctx, func(cfg auth.Config) auth.Config {
			cfg.DBProvider = func(ctx context.Context) (authdb.DB, error) {
				return authdb.NewSnapshotDB(&protocol.AuthDB{
					OauthClientId: "Client ID",
					Groups: []*protocol.AuthGroup{
						{
							Name:    auth.InternalServicesGroup,
							Members: []string{"user:service@example.com"},
						},
						{
							Name:    "user-group",
							Members: []string{"user:someone@example.com"},
						},
					},
				}, "http://auth.example.com", 1234, false)
			}
			return cfg
		})

		srv := &authServerImpl{
			info: &sidecar.ServerInfo{
				SidecarService: "service",
				SidecarJob:     "job",
				SidecarHost:    "host",
				SidecarVersion: "version",
			},
		}

		expectedInfo := &sidecar.ServerInfo{
			SidecarService: "service",
			SidecarJob:     "job",
			SidecarHost:    "host",
			SidecarVersion: "version",
			AuthDbService:  "http://auth.example.com",
			AuthDbRev:      1234,
		}

		mockAuthUser := func(u *auth.User) {
			srv.authenticator = auth.Authenticator{
				Methods: []auth.Method{
					authtest.FakeAuth{
						User: u,
					},
				},
			}
		}

		mockAuthError := func(err error) {
			srv.authenticator = auth.Authenticator{
				Methods: []auth.Method{
					authtest.FakeAuth{
						Error: err,
					},
				},
			}
		}

		call := func(md ...*sidecar.AuthenticateRequest_Metadata) (*sidecar.AuthenticateResponse, error) {
			return srv.Authenticate(ctx, &sidecar.AuthenticateRequest{
				Protocol: sidecar.AuthenticateRequest_HTTP1,
				Metadata: md,
			})
		}

		t.Run("Anonymous", func(t *ftt.Test) {
			mockAuthUser(&auth.User{Identity: identity.AnonymousIdentity})
			res, err := call()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&sidecar.AuthenticateResponse{
				Identity:   "anonymous:anonymous",
				ServerInfo: expectedInfo,
				Outcome: &sidecar.AuthenticateResponse_Anonymous_{
					Anonymous: &sidecar.AuthenticateResponse_Anonymous{},
				},
			}))
		})

		t.Run("User", func(t *ftt.Test) {
			mockAuthUser(&auth.User{
				Identity: "user:someone@example.com",
				Email:    "someone@example.com",
				Name:     "Full Name",
				Picture:  "Picture",
				ClientID: "Client ID",
			})
			res, err := call()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&sidecar.AuthenticateResponse{
				Identity:   "user:someone@example.com",
				ServerInfo: expectedInfo,
				Outcome: &sidecar.AuthenticateResponse_User_{
					User: &sidecar.AuthenticateResponse_User{
						Email:    "someone@example.com",
						Name:     "Full Name",
						Picture:  "Picture",
						ClientId: "Client ID",
					},
				},
			}))
		})

		t.Run("Project", func(t *ftt.Test) {
			mockAuthUser(&auth.User{Identity: "user:service@example.com"})
			res, err := call(&sidecar.AuthenticateRequest_Metadata{
				Key:   auth.XLUCIProjectHeader,
				Value: "something",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&sidecar.AuthenticateResponse{
				Identity:   "project:something",
				ServerInfo: expectedInfo,
				Outcome: &sidecar.AuthenticateResponse_Project_{
					Project: &sidecar.AuthenticateResponse_Project{
						Project: "something",
						Service: "user:service@example.com",
					},
				},
			}))
		})

		t.Run("Unknown identity kind", func(t *ftt.Test) {
			mockAuthUser(&auth.User{Identity: "bot:what"})
			res, err := call()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&sidecar.AuthenticateResponse{
				Identity:   "anonymous:anonymous",
				ServerInfo: expectedInfo,
				Outcome: &sidecar.AuthenticateResponse_Error{
					Error: &statuspb.Status{
						Code:    int32(codes.Unauthenticated),
						Message: "request was authenticated as \"bot:what\" which is an identity kind not supported by the LUCI Sidecar server",
					},
				},
			}))
		})

		t.Run("Fatal auth error", func(t *ftt.Test) {
			mockAuthError(fmt.Errorf("boom"))
			res, err := call()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&sidecar.AuthenticateResponse{
				Identity:   "anonymous:anonymous",
				ServerInfo: expectedInfo,
				Outcome: &sidecar.AuthenticateResponse_Error{
					Error: &statuspb.Status{
						Code:    int32(codes.Unauthenticated),
						Message: "boom",
					},
				},
			}))
		})

		t.Run("Fatal auth error with code", func(t *ftt.Test) {
			mockAuthError(status.Errorf(codes.PermissionDenied, "boom"))
			res, err := call()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&sidecar.AuthenticateResponse{
				Identity:   "anonymous:anonymous",
				ServerInfo: expectedInfo,
				Outcome: &sidecar.AuthenticateResponse_Error{
					Error: &statuspb.Status{
						Code:    int32(codes.PermissionDenied),
						Message: "boom",
					},
				},
			}))
		})

		t.Run("Transient error", func(t *ftt.Test) {
			mockAuthError(errors.New("boom", transient.Tag))
			_, err := call()
			assert.Loosely(t, err, grpccode.ShouldBe(codes.Internal))
			assert.Loosely(t, err, should.ErrLike("boom"))
		})

		t.Run("Group check", func(t *ftt.Test) {
			mockAuthUser(&auth.User{
				Identity: "user:someone@example.com",
				Email:    "someone@example.com",
			})
			res, err := srv.Authenticate(ctx, &sidecar.AuthenticateRequest{
				Protocol: sidecar.AuthenticateRequest_HTTP1,
				Groups:   []string{"user-group", "something-else"},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&sidecar.AuthenticateResponse{
				Identity:   "user:someone@example.com",
				ServerInfo: expectedInfo,
				Groups:     []string{"user-group"},
				Outcome: &sidecar.AuthenticateResponse_User_{
					User: &sidecar.AuthenticateResponse_User{
						Email: "someone@example.com",
					},
				},
			}))
		})
	})
}

func TestIsMember(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx := auth.WithState(context.Background(), &authtest.FakeState{
			Identity: "user:sidecar-user-unused@example.com",
			FakeDB: authtest.NewFakeDB(
				authtest.MockMembership("user:enduser@example.com", "group-1"),
				authtest.MockMembership("user:sidecar-user-unused@example.com", "group-2"),
			),
		})

		srv := &authServerImpl{}

		t.Run("OK", func(t *ftt.Test) {
			res, err := srv.IsMember(ctx, &sidecar.IsMemberRequest{
				Identity: "user:enduser@example.com",
				Groups:   []string{"group-1", "group-2"},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.IsMember, should.BeTrue)
		})

		t.Run("Not a member", func(t *ftt.Test) {
			res, err := srv.IsMember(ctx, &sidecar.IsMemberRequest{
				Identity: "user:enduser@example.com",
				Groups:   []string{"group-2"},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.IsMember, should.BeFalse)
		})

		t.Run("No ident", func(t *ftt.Test) {
			_, err := srv.IsMember(ctx, &sidecar.IsMemberRequest{
				Groups: []string{"group-2"},
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("identity field is required"))
		})

		t.Run("Bad ident", func(t *ftt.Test) {
			_, err := srv.IsMember(ctx, &sidecar.IsMemberRequest{
				Identity: "what",
				Groups:   []string{"group-2"},
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("bad identity"))
		})

		t.Run("No groups", func(t *ftt.Test) {
			_, err := srv.IsMember(ctx, &sidecar.IsMemberRequest{
				Identity: "user:enduser@example.com",
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("at least one group is required"))
		})
	})
}

func TestHasPermission(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx := auth.WithState(context.Background(), &authtest.FakeState{
			Identity: "user:sidecar-user-unused@example.com",
			FakeDB: authtest.NewFakeDB(
				authtest.MockPermission("user:enduser@example.com", "test:realm", testPerm0),
				authtest.MockPermission("user:enduser@example.com", "test:realm", testPerm1,
					authtest.RestrictAttribute("test.attr", "good-val"),
				),
			),
		})

		srv := &authServerImpl{
			perms: map[string]realms.Permission{
				testPerm0.Name(): testPerm0,
				testPerm1.Name(): testPerm1,
			},
		}

		t.Run("OK", func(t *ftt.Test) {
			res, err := srv.HasPermission(ctx, &sidecar.HasPermissionRequest{
				Identity:   "user:enduser@example.com",
				Permission: testPerm0.Name(),
				Realm:      "test:realm",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.HasPermission, should.BeTrue)
		})

		t.Run("OK with attrs", func(t *ftt.Test) {
			res, err := srv.HasPermission(ctx, &sidecar.HasPermissionRequest{
				Identity:   "user:enduser@example.com",
				Permission: testPerm1.Name(),
				Realm:      "test:realm",
				Attributes: map[string]string{"test.attr": "good-val"},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.HasPermission, should.BeTrue)
		})

		t.Run("No permission", func(t *ftt.Test) {
			res, err := srv.HasPermission(ctx, &sidecar.HasPermissionRequest{
				Identity:   "user:enduser@example.com",
				Permission: testPerm1.Name(),
				Realm:      "test:realm",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.HasPermission, should.BeFalse)
		})

		t.Run("No ident", func(t *ftt.Test) {
			_, err := srv.HasPermission(ctx, &sidecar.HasPermissionRequest{
				Permission: testPerm0.Name(),
				Realm:      "test:realm",
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("identity field is required"))
		})

		t.Run("Bad ident", func(t *ftt.Test) {
			_, err := srv.HasPermission(ctx, &sidecar.HasPermissionRequest{
				Identity:   "what",
				Permission: testPerm0.Name(),
				Realm:      "test:realm",
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("bad identity"))
		})

		t.Run("No perm", func(t *ftt.Test) {
			_, err := srv.HasPermission(ctx, &sidecar.HasPermissionRequest{
				Identity: "user:enduser@example.com",
				Realm:    "test:realm",
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("permission field is required"))
		})

		t.Run("Bad perm", func(t *ftt.Test) {
			_, err := srv.HasPermission(ctx, &sidecar.HasPermissionRequest{
				Identity:   "user:enduser@example.com",
				Permission: "what",
				Realm:      "test:realm",
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("bad permission"))
		})

		t.Run("Unknown perm", func(t *ftt.Test) {
			_, err := srv.HasPermission(ctx, &sidecar.HasPermissionRequest{
				Identity:   "user:enduser@example.com",
				Permission: "fake.permission.unknown",
				Realm:      "test:realm",
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("is not registered"))
		})

		t.Run("No realm", func(t *ftt.Test) {
			_, err := srv.HasPermission(ctx, &sidecar.HasPermissionRequest{
				Identity:   "user:enduser@example.com",
				Permission: testPerm0.Name(),
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("realm field is required"))
		})

		t.Run("Bad realm", func(t *ftt.Test) {
			_, err := srv.HasPermission(ctx, &sidecar.HasPermissionRequest{
				Identity:   "user:enduser@example.com",
				Permission: testPerm0.Name(),
				Realm:      "bad",
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("bad global realm name"))
		})
	})
}

func TestRequestMetadata(t *testing.T) {
	t.Parallel()

	ftt.Run("HTTP", t, func(t *ftt.Test) {
		req, err := newRequestMetadata(&sidecar.AuthenticateRequest{
			Protocol: sidecar.AuthenticateRequest_HTTP1,
			Metadata: []*sidecar.AuthenticateRequest_Metadata{
				{Key: "Header", Value: "Val1"},
				{Key: "HeaDer", Value: "Val2"},
				{Key: "Host", Value: "host"},
				{Key: "Header-Bin", Value: "val"},
				{Key: "Cookie", Value: "cookie_1=value_1; cookie_2=value_2"},
				{Key: "Cookie", Value: "cookie_3=value_3"},
			},
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, req.Host(), should.Equal("host"))
		assert.Loosely(t, req.RemoteAddr(), should.BeEmpty)
		assert.Loosely(t, req.Header("HEADER"), should.Equal("Val1"))
		assert.Loosely(t, req.Header("header-bin"), should.Equal("val"))

		cookie, err := req.Cookie("cookie_3")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cookie.Value, should.Equal("value_3"))
	})

	ftt.Run("gRPC", t, func(t *ftt.Test) {
		req, err := newRequestMetadata(&sidecar.AuthenticateRequest{
			Protocol: sidecar.AuthenticateRequest_GRPC,
			Metadata: []*sidecar.AuthenticateRequest_Metadata{
				{Key: ":authority", Value: "host"},
				{Key: "Header-Bin", Value: base64.RawStdEncoding.EncodeToString([]byte("val"))},
			},
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, req.Host(), should.Equal("host"))
		assert.Loosely(t, req.Header("header-bin"), should.Equal("val"))
	})

	ftt.Run("Unknown protocol", t, func(t *ftt.Test) {
		_, err := newRequestMetadata(&sidecar.AuthenticateRequest{})
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
	})
}
