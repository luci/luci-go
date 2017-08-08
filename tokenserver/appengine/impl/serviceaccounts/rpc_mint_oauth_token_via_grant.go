// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package serviceaccounts

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/tokenserver/api/minter/v1"
)

// MintOAuthTokenViaGrantRPC implements TokenMinter.MintOAuthTokenViaGrant
// method.
type MintOAuthTokenViaGrantRPC struct {
}

// MintOAuthTokenViaGrant produces new OAuth token given a grant.
func (r *MintOAuthTokenViaGrantRPC) MintOAuthTokenViaGrant(c context.Context, req *minter.MintOAuthTokenViaGrantRequest) (*minter.MintOAuthTokenViaGrantResponse, error) {
	return nil, grpc.Errorf(codes.Unavailable, "not implemented")
}
