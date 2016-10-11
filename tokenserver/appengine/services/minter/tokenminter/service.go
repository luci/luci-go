// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package tokenminter implements TokenMinter API.
//
// This is main public API of The Token Server.
package tokenminter

import (
	"github.com/luci/luci-go/appengine/gaeauth/server/gaesigner"

	"github.com/luci/luci-go/tokenserver/appengine/certchecker"
	"github.com/luci/luci-go/tokenserver/appengine/delegation"
	"github.com/luci/luci-go/tokenserver/appengine/machinetoken"
)

// Server implements minter.TokenMinterServer RPC interface.
//
// This is just an assembly of individual method implementations, properly
// configured for use in GAE prod setting.
//
// Use NewServer to make a new instance.
type Server struct {
	machinetoken.MintMachineTokenRPC
	delegation.MintDelegationTokenRPC
}

// NewServer returns Server configured for real production usage.
func NewServer() *Server {
	return &Server{
		MintMachineTokenRPC: machinetoken.MintMachineTokenRPC{
			Signer:           gaesigner.Signer{},
			CheckCertificate: certchecker.CheckCertificate,
		},
		MintDelegationTokenRPC: delegation.MintDelegationTokenRPC{
			Signer: gaesigner.Signer{},
		},
	}
}
