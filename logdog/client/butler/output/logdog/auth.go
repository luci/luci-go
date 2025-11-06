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

package logdog

import (
	"context"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/lucictx"
)

// Auth incapsulates an authentication scheme to use.
//
// Construct it using either LegacyAuth() or RealmsAwareAuth().
type Auth interface {
	// Project is the current project taken from LUCI_CONTEXT (or "").
	Project() string
	// Realm is the current realm taken from LUCI_CONTEXT (or "").
	Realm() string
	// RPC returns an authenticator to use for RegisterPrefix RPC call.
	RPC() *auth.Authenticator
	// PubSub returns an authenticator to use for PubSub calls.
	PubSub() *auth.Authenticator
}

// LegacyAuth returns an authentication scheme with pre-realms logic.
//
// Will be eventually removed once all call sites are aware of realms.
func LegacyAuth(a *auth.Authenticator) Auth {
	return legacyAuth{a}
}

type legacyAuth struct {
	a *auth.Authenticator
}

func (la legacyAuth) Project() string             { return "" }
func (la legacyAuth) Realm() string               { return "" }
func (la legacyAuth) RPC() *auth.Authenticator    { return la.a }
func (la legacyAuth) PubSub() *auth.Authenticator { return la.a }

// RealmsAwareAuth returns an authentication scheme with realms-aware logic.
//
// The given context will be used to:
//  1. Grab the current realm name from LUCI_CONTEXT.
//  2. Construct a transport that uses the default account for auth (thus the
//     context should have the default task account selected in it).
//  3. Switch into "system" local account and construct a transport that uses
//     this account for PubSub calls.
//
// If there's no realm in the context, degrades to LegacyAuth using the "system"
// local account for all calls, as was the way before the realms mode.
func RealmsAwareAuth(ctx context.Context) (Auth, error) {
	sysCtx, err := lucictx.SwitchLocalAccount(ctx, "system")
	if err != nil {
		return nil, errors.Fmt("could not switch to 'system' account in LUCI_CONTEXT: %w", err)
	}
	sysAuth := auth.NewAuthenticator(sysCtx, auth.SilentLogin, auth.Options{
		Scopes:    scopes.CloudScopeSet(),
		MonitorAs: "logdog/system",
	})

	project, realm := lucictx.CurrentRealm(ctx)
	if realm == "" {
		return LegacyAuth(sysAuth), nil
	}

	return &realmsAuth{
		project: project,
		realm:   realm,
		rpc: auth.NewAuthenticator(ctx, auth.SilentLogin, auth.Options{
			Scopes:    scopes.DefaultScopeSet(),
			MonitorAs: "logdog/rpc",
		}),
		pubSub: sysAuth,
	}, nil
}

type realmsAuth struct {
	project string
	realm   string
	rpc     *auth.Authenticator
	pubSub  *auth.Authenticator
}

func (ra *realmsAuth) Project() string             { return ra.project }
func (ra *realmsAuth) Realm() string               { return ra.realm }
func (ra *realmsAuth) RPC() *auth.Authenticator    { return ra.rpc }
func (ra *realmsAuth) PubSub() *auth.Authenticator { return ra.pubSub }
