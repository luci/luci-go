// Copyright 2016 The LUCI Authors.
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

// Package tokenminter implements TokenMinter API.
//
// This is main public API of The Token Server.
package tokenminter

import (
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/signing"

	"go.chromium.org/luci/tokenserver/api/minter/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/certchecker"
	"go.chromium.org/luci/tokenserver/appengine/impl/delegation"
	"go.chromium.org/luci/tokenserver/appengine/impl/machinetoken"
	"go.chromium.org/luci/tokenserver/appengine/impl/projectscope"
	"go.chromium.org/luci/tokenserver/appengine/impl/serviceaccounts"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/projectidentity"
)

// serverImpl implements minter.TokenMinterServer RPC interface.
type serverImpl struct {
	minter.UnsafeTokenMinterServer

	machinetoken.MintMachineTokenRPC
	delegation.MintDelegationTokenRPC
	projectscope.MintProjectTokenRPC
	serviceaccounts.MintServiceAccountTokenRPC
}

// NewServer returns prod TokenMinterServer implementation.
//
// It does all authorization checks inside.
func NewServer(signer signing.Signer, prod bool) minter.TokenMinterServer {
	return &serverImpl{
		MintMachineTokenRPC: machinetoken.MintMachineTokenRPC{
			Signer:           signer,
			CheckCertificate: certchecker.CheckCertificate,
			LogToken:         machinetoken.NewTokenLogger(!prod),
		},
		MintDelegationTokenRPC: delegation.MintDelegationTokenRPC{
			Signer:   signer,
			Rules:    delegation.GlobalRulesCache.Rules,
			LogToken: delegation.NewTokenLogger(!prod),
		},
		MintProjectTokenRPC: projectscope.MintProjectTokenRPC{
			Signer:            signer,
			MintAccessToken:   auth.MintAccessTokenForServiceAccount,
			ProjectIdentities: projectidentity.ProjectIdentities,
			LogToken:          projectscope.NewTokenLogger(!prod),
		},
		MintServiceAccountTokenRPC: serviceaccounts.MintServiceAccountTokenRPC{
			Signer:          signer,
			Mapping:         serviceaccounts.GlobalMappingCache.Mapping,
			MintAccessToken: auth.MintAccessTokenForServiceAccount,
			MintIDToken:     auth.MintIDTokenForServiceAccount,
			LogToken:        serviceaccounts.NewTokenLogger(!prod),
		},
	}
}
