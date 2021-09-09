// Copyright 2021 The LUCI Authors.
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

// Package internals contains Internals server implementation.
package internals

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth/xsrf"

	"go.chromium.org/luci/auth_service/api/internalspb"
)

// Server implements Internals server.
type Server struct {
	internalspb.UnimplementedInternalsServer
}

// RefreshXSRFToken implements the corresponding RPC method.
func (*Server) RefreshXSRFToken(ctx context.Context, req *internalspb.RefreshXSRFTokenRequest) (*internalspb.XSRFToken, error) {
	switch err := xsrf.Check(ctx, req.XsrfToken); {
	case transient.Tag.In(err):
		return nil, status.Errorf(codes.Internal, "transient error when checking the XSRF token: %s", err)
	case err != nil:
		return nil, status.Errorf(codes.PermissionDenied, "bad XSRF token: %s", err)
	}

	refreshed, err := xsrf.Token(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate new XSRF token: %s", err)
	}

	return &internalspb.XSRFToken{XsrfToken: refreshed}, nil
}
