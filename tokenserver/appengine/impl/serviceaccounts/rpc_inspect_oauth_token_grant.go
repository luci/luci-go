// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package serviceaccounts

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/luci/luci-go/tokenserver/api/admin/v1"
)

// InspectOAuthTokenGrantRPC implements Admin.InspectOAuthTokenGrant method.
type InspectOAuthTokenGrantRPC struct {
}

// InspectOAuthTokenGrant decodes the given OAuth token grant.
func (r *ImportServiceAccountsConfigsRPC) InspectOAuthTokenGrant(c context.Context, req *admin.InspectOAuthTokenGrantRequest) (*admin.InspectOAuthTokenGrantResponse, error) {
	return nil, grpc.Errorf(codes.Unavailable, "not implemented")
}
