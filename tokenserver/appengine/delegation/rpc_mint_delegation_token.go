// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package delegation

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/luci/luci-go/server/auth/signing"
	minter "github.com/luci/luci-go/tokenserver/api/minter/v1"
)

// MintDelegationTokenRPC implements TokenMinter.MintDelegationToken RPC method.
type MintDelegationTokenRPC struct {
	// Signer is mocked in tests.
	//
	// In prod it is gaesigner.Signer.
	Signer signing.Signer
}

// MintDelegationToken generates a new bearer delegation token.
func (r *MintDelegationTokenRPC) MintDelegationToken(c context.Context, req *minter.MintDelegationTokenRequest) (*minter.MintDelegationTokenResponse, error) {
	return nil, grpc.Errorf(codes.Unimplemented, "Not implemented yet")
}
