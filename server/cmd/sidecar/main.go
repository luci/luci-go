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
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/flag/stringlistflag"
	"go.chromium.org/luci/common/proto/sidecar"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/auth/realms"
)

func main() {
	var permissions stringlistflag.Flag
	var openIDAudience stringlistflag.Flag

	flag.Var(
		&permissions,
		"sidecar-subscribe-to-permission",
		"A permission that will be accepted by HasPermission RPC",
	)
	flag.Var(
		&openIDAudience,
		"sidecar-open-id-rpc-auth-audience",
		"Additional accepted value of `aud` claim in OpenID tokens, can be repeated",
	)

	// Prepare server options by probing environment and parsing flags. This
	// normally happens in server.Main(...), but here we can't use it, see the
	// comment below.
	opts, err := server.OptionsFromEnv(&server.Options{
		DefaultRequestTimeout: 15 * time.Second, // all requests should be super fast
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "When constructing options: %s\n", err)
		os.Exit(3)
	}
	opts.Register(flag.CommandLine)
	flag.Parse()
	if args := flag.Args(); len(args) > 0 {
		fmt.Fprintf(os.Stderr, "got unexpected positional command line arguments: %v\n", args)
		os.Exit(3)
	}

	// Register all permissions as soon as possible. Normally they are registered
	// during init() time, but here this list is dynamic and we need to do it
	// after flags are parsed. Note that doing it after server.New(...) call is
	// already too late, since server.New(...) fetches AuthDB which uses
	// registered permissions already. Registering them after server.New(...)
	// causes a panic.
	perms := make(map[string]realms.Permission, len(permissions))
	for _, perm := range permissions {
		if err := realms.ValidatePermissionName(perm); err != nil {
			fmt.Fprintf(os.Stderr, "Bad -sidecar-subscribe-to-permission: %s\n", err)
			os.Exit(3)
		}
		perms[perm] = realms.RegisterPermission(perm)
	}

	// Finish initializing the server.
	srv, err := server.New(context.Background(), *opts, nil)
	if err != nil {
		srv.Fatal(err)
	}
	sidecar.RegisterAuthServer(srv, &authServerImpl{
		// Details about the server returned to clients.
		info: &sidecar.ServerInfo{
			SidecarService: srv.Options.TsMonServiceName,
			SidecarJob:     srv.Options.TsMonJobName,
			SidecarHost:    srv.Options.Hostname,
			SidecarVersion: srv.Options.ImageVersion(),
		},
		// Authentication methods used by Authenticate RPC to authenticate
		// end-user requests. Note that there's a separate stack of auth methods
		// used to authenticate RPCs to the sidecar server itself. It is
		// configured by server.Server as usual.
		authenticator: auth.Authenticator{
			Methods: []auth.Method{
				// Preferred method using OpenID identity tokens (JWTs).
				&openid.GoogleIDTokenAuthMethod{
					AudienceCheck: openid.AudienceMatchesHost,
					Audience:      openIDAudience,
					SkipNonJWT:    true, // pass OAuth2 access tokens through
				},
				// Fallback method to support Google OAuth2 access tokens. Slow.
				&auth.GoogleOAuth2Method{
					Scopes: []string{"https://www.googleapis.com/auth/userinfo.email"},
				},
			},
		},
		// Map permission name => realms.Permission.
		perms: perms,
	})

	// Run the serving loop.
	if err = srv.Serve(); err != nil {
		srv.Fatal(err)
	}
}

type authServerImpl struct {
	sidecar.UnimplementedAuthServer

	info          *sidecar.ServerInfo
	authenticator auth.Authenticator
	perms         map[string]realms.Permission
}

// serverInfo returns information about the sidecar server to put into replies.
func (s *authServerImpl) serverInfo(ctx context.Context) *sidecar.ServerInfo {
	if s.info == nil {
		return nil
	}
	info := &sidecar.ServerInfo{
		SidecarService: s.info.SidecarService,
		SidecarJob:     s.info.SidecarJob,
		SidecarHost:    s.info.SidecarHost,
		SidecarVersion: s.info.SidecarVersion,
	}
	db := auth.GetState(ctx).DB()
	info.AuthDbService, _ = db.GetAuthServiceURL(ctx)
	info.AuthDbRev = authdb.Revision(db)
	return info
}

// Authenticate implements corresponding RPC method.
func (s *authServerImpl) Authenticate(ctx context.Context, req *sidecar.AuthenticateRequest) (*sidecar.AuthenticateResponse, error) {
	reqMeta, err := newRequestMetadata(req)
	if err != nil {
		return nil, err
	}

	rctx, err := s.authenticator.Authenticate(
		auth.ModifyConfig(ctx, func(cfg auth.Config) auth.Config {
			// This tells the auth library to use req.RemoteAddr().
			cfg.EndUserIP = nil
			// Do not expose frontend client ID of the side car server itself.
			cfg.FrontendClientID = nil
			return cfg
		}), reqMeta,
	)

	if err != nil {
		// Find the statuspb.Status if available. Otherwise use LUCI error tags.
		statuspb, ok := status.FromError(err)
		if !ok {
			code, ok := grpcutil.Tag.In(err)
			if !ok {
				if transient.Tag.In(err) {
					code = codes.Internal
				} else {
					code = codes.Unauthenticated
				}
			}
			statuspb = status.New(code, err.Error())
		}
		// Return internal errors as overall RPC errors to trigger a retry.
		if grpcutil.IsTransientCode(statuspb.Code()) {
			return nil, statuspb.Err()
		}
		// The rest is returned as a successful RPC reply that carries
		// an authentication error inside as a payload. `Authenticate` RPC itself
		// succeeded, it is passed credentials which are broken. Returning errors
		// this way (instead of just returning them as overall RPC status) allows to
		// distinguish invalid credentials inside AuthenticateResponse from invalid
		// credentials of the RPC itself.
		return &sidecar.AuthenticateResponse{
			Identity:   string(identity.AnonymousIdentity),
			ServerInfo: s.serverInfo(ctx),
			Outcome: &sidecar.AuthenticateResponse_Error{
				Error: statuspb.Proto(),
			},
		}, nil
	}

	user := auth.CurrentUser(rctx)
	resp := &sidecar.AuthenticateResponse{
		Identity:   string(user.Identity),
		ServerInfo: s.serverInfo(rctx),
	}

	switch user.Identity.Kind() {
	case identity.Anonymous:
		resp.Outcome = &sidecar.AuthenticateResponse_Anonymous_{
			Anonymous: &sidecar.AuthenticateResponse_Anonymous{},
		}
	case identity.User:
		resp.Outcome = &sidecar.AuthenticateResponse_User_{
			User: &sidecar.AuthenticateResponse_User{
				Email:    user.Email,
				Name:     user.Name,
				Picture:  user.Picture,
				ClientId: user.ClientID,
			},
		}
	case identity.Project:
		resp.Outcome = &sidecar.AuthenticateResponse_Project_{
			Project: &sidecar.AuthenticateResponse_Project{
				Project: user.Identity.Value(),
				Service: string(auth.GetState(rctx).PeerIdentity()),
			},
		}
	default:
		resp.Identity = string(identity.AnonymousIdentity)
		resp.Outcome = &sidecar.AuthenticateResponse_Error{
			Error: status.Newf(codes.Unauthenticated,
				"request was authenticated as %q which is an identity kind not "+
					"supported by the LUCI Sidecar server", user.Identity,
			).Proto(),
		}
		return resp, nil
	}

	if len(req.Groups) != 0 {
		resp.Groups, err = auth.GetState(ctx).DB().CheckMembership(ctx, user.Identity, req.Groups)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to check groups membership: %s", err)
		}
	}

	return resp, nil
}

// IsMember implements corresponding RPC method.
func (s *authServerImpl) IsMember(ctx context.Context, req *sidecar.IsMemberRequest) (*sidecar.IsMemberResponse, error) {
	if req.Identity == "" {
		return nil, status.Errorf(codes.InvalidArgument, "identity field is required")
	}
	if len(req.Groups) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "at least one group is required")
	}
	ident, err := identity.MakeIdentity(req.Identity)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad identity: %s", err)
	}
	yes, err := auth.GetState(ctx).DB().IsMember(ctx, ident, req.Groups)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to check groups membership: %s", err)
	}
	return &sidecar.IsMemberResponse{
		IsMember:   yes,
		ServerInfo: s.serverInfo(ctx),
	}, nil
}

// HasPermission implements corresponding RPC method.
func (s *authServerImpl) HasPermission(ctx context.Context, req *sidecar.HasPermissionRequest) (*sidecar.HasPermissionResponse, error) {
	if req.Identity == "" {
		return nil, status.Errorf(codes.InvalidArgument, "identity field is required")
	}
	ident, err := identity.MakeIdentity(req.Identity)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad identity: %s", err)
	}

	if req.Permission == "" {
		return nil, status.Errorf(codes.InvalidArgument, "permission field is required")
	}
	perm, ok := s.perms[req.Permission]
	if !ok {
		if err := realms.ValidatePermissionName(req.Permission); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "bad permission: %s", err)
		}
		return nil, status.Errorf(codes.InvalidArgument, "bad permission: %q is not registered with the sidecar server via -sidecar-subscribe-to-permission", req.Permission)
	}

	if req.Realm == "" {
		return nil, status.Errorf(codes.InvalidArgument, "realm field is required")
	}
	if err := realms.ValidateRealmName(req.Realm, realms.GlobalScope); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad realm: %s", err)
	}

	yes, err := auth.GetState(ctx).DB().HasPermission(ctx, ident, perm, req.Realm, req.Attributes)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to check permission: %s", err)
	}
	return &sidecar.HasPermissionResponse{
		HasPermission: yes,
		ServerInfo:    s.serverInfo(ctx),
	}, nil
}

////////////////////////////////////////////////////////////////////////////////

// requestMetadata implements auth.RequestMetadata via decoded request metadata.
type requestMetadata map[string][]string

func newRequestMetadata(req *sidecar.AuthenticateRequest) (requestMetadata, error) {
	if req.Protocol == 0 || req.Protocol > sidecar.AuthenticateRequest_GRPC {
		return nil, status.Errorf(codes.InvalidArgument, "unknown protocol #%d", req.Protocol)
	}

	md := make(requestMetadata, len(req.Metadata))
	for _, kv := range req.Metadata {
		key := strings.ToLower(kv.Key)
		val := kv.Value
		if req.Protocol == sidecar.AuthenticateRequest_GRPC && strings.HasSuffix(key, "-bin") {
			blob, err := decodeBinMetadata(val)
			if err != nil {
				return requestMetadata{}, status.Errorf(codes.InvalidArgument, "bad binary metadata %q: %s", key, err)
			}
			val = string(blob)
		}
		md[key] = append(md[key], val)
	}

	// Normalize to use HTTP2 pseudo-headers for Host.
	if req.Protocol == sidecar.AuthenticateRequest_HTTP1 {
		if val, ok := md["host"]; ok {
			md[":authority"] = val
			delete(md, "host")
		}
	}

	return md, nil
}

func (r requestMetadata) Host() string {
	return r.Header(":authority")
}

func (r requestMetadata) RemoteAddr() string {
	// TODO(vadimsh): Extract from X-Forwarded-For or equivalent if necessary.
	// This will require exposing a command line flag which tells what parts of
	// X-Forwarded-For can be trusted.
	return ""
}

func (r requestMetadata) Header(key string) string {
	if vals := r[strings.ToLower(key)]; len(vals) != 0 {
		return vals[0]
	}
	return ""
}

func (r requestMetadata) Cookie(key string) (*http.Cookie, error) {
	cookies := r["cookie"]
	if len(cookies) == 0 {
		return nil, http.ErrNoCookie
	}
	return (&http.Request{Header: http.Header{"Cookie": cookies}}).Cookie(key)
}

func decodeBinMetadata(v string) ([]byte, error) {
	if len(v)%4 == 0 {
		// Input was padded, or padding was not necessary.
		return base64.StdEncoding.DecodeString(v)
	}
	return base64.RawStdEncoding.DecodeString(v)
}
