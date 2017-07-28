// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package serviceaccounts

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/luci/luci-go/tokenserver/api/minter/v1"
)

// MintOAuthTokenGrantRPC implements TokenMinter.MintOAuthTokenGrant method.
type MintOAuthTokenGrantRPC struct {
}

// MintOAuthTokenGrant produces new OAuth token grant.
func (r *MintOAuthTokenGrantRPC) MintOAuthTokenGrant(c context.Context, req *minter.MintOAuthTokenGrantRequest) (*minter.MintOAuthTokenGrantResponse, error) {
	return nil, grpc.Errorf(codes.Unavailable, "not implemented")
}
