// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package lucictx

import (
	"fmt"

	"golang.org/x/net/context"
)

// LocalAuth is a struct that may be used with the "local_auth" section of
// LUCI_CONTEXT.
type LocalAuth struct {
	// RPCPort and Secret define how to connect to the local auth server.
	RPCPort uint32 `json:"rpc_port"`
	Secret  []byte `json:"secret"`

	// Accounts and DefaultAccountID defines what access tokens are available.
	Accounts         []LocalAuthAccount `json:"accounts"`
	DefaultAccountID string             `json:"default_account_id"`
}

// LocalAuthAccount contains information about a service account available
// through a local auth server.
type LocalAuthAccount struct {
	// ID is logical identifier of the account, e.g. "system" or "task".
	ID string `json:"id"`
}

// CanUseByDefault returns true if the authentication context can be picked up
// by default.
//
// TODO(vadimsh): Remove this method once all servers provide 'accounts'.
func (la *LocalAuth) CanUseByDefault() bool {
	// Old API servers don't provide list of accounts. Instead there's single
	// account that is always used by default.
	if len(la.Accounts) == 0 {
		return true
	}
	// New API servers give a list of available account and an optional default
	// account. Auth should be used only if default account is given.
	return la.DefaultAccountID != ""
}

// GetLocalAuth calls Lookup and returns the current LocalAuth from LUCI_CONTEXT
// if it was present. If no LocalAuth is in the context, this returns nil.
func GetLocalAuth(ctx context.Context) *LocalAuth {
	ret := LocalAuth{}
	ok, err := Lookup(ctx, "local_auth", &ret)
	if err != nil {
		panic(err)
	}
	if !ok {
		return nil
	}
	return &ret
}

// SetLocalAuth Sets the LocalAuth in the LUCI_CONTEXT.
func SetLocalAuth(ctx context.Context, la *LocalAuth) context.Context {
	ctx, err := Set(ctx, "local_auth", la)
	if err != nil {
		panic(fmt.Errorf("impossible: %s", err))
	}
	return ctx
}
