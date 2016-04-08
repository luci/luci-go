// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package tokenminter implements TokenMinter API.
//
// This is main public API of The Token Server.
package tokenminter

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/luci/luci-go/common/api/tokenserver/v1"
)

// Server implements tokenserver.TokenMinterServer RPC interface.
type Server struct {
}

// MintToken generates a new token for an authenticated caller.
func (s *Server) MintToken(c context.Context, req *tokenserver.MintTokenRequest) (*tokenserver.MintTokenResponse, error) {
	return nil, grpc.Errorf(codes.Unimplemented, "not implemented")
}
