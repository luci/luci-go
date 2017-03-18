// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package tokenminter implements TokenMinter API.
//
// This is main public API of The Token Server.
package tokenminter

import (
	"github.com/luci/luci-go/appengine/gaeauth/server/gaesigner"

	"github.com/luci/luci-go/tokenserver/appengine/impl/certchecker"
	"github.com/luci/luci-go/tokenserver/appengine/impl/delegation"
	"github.com/luci/luci-go/tokenserver/appengine/impl/machinetoken"

	"github.com/luci/luci-go/tokenserver/api/minter/v1"
)

// Server implements minter.TokenMinterServer RPC interface.
//
// This is just an assembly of individual method implementations, properly
// configured for use in GAE prod setting.
type serverImpl struct {
	machinetoken.MintMachineTokenRPC
	delegation.MintDelegationTokenRPC
}

// NewServer returns prod TokenMinterServer implementation.
//
// It does all authorization checks inside.
func NewServer() minter.TokenMinterServer {
	return &serverImpl{
		MintMachineTokenRPC: machinetoken.MintMachineTokenRPC{
			Signer:           gaesigner.Signer{},
			CheckCertificate: certchecker.CheckCertificate,
			LogToken:         machinetoken.LogToken,
		},
		MintDelegationTokenRPC: delegation.MintDelegationTokenRPC{
			Signer:       gaesigner.Signer{},
			ConfigLoader: delegation.DelegationConfigLoader(),
			LogToken:     delegation.LogToken,
		},
	}
}
