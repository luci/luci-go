// Copyright 2024 The LUCI Authors.
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

// Package perms defines permissions used to control access to Tree Status
// resources, and related methods.
package perms

import (
	"context"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"
)

// FakeAuthBuilder creates some fake identity, only used for testing.
type FakeAuthBuilder struct {
	state *authtest.FakeState
}

func FakeAuth() *FakeAuthBuilder {
	return &FakeAuthBuilder{
		state: &authtest.FakeState{
			Identity:            "user:someone@example.com",
			IdentityGroups:      []string{},
			IdentityPermissions: []authtest.RealmPermission{},
		},
	}
}
func (a *FakeAuthBuilder) Anonymous() *FakeAuthBuilder {
	a.state.Identity = "anonymous:anonymous"
	return a
}
func (a *FakeAuthBuilder) WithReadAccess() *FakeAuthBuilder {
	a.state.IdentityGroups = append(a.state.IdentityGroups, treeStatusAccessGroup)
	return a
}
func (a *FakeAuthBuilder) WithAuditAccess() *FakeAuthBuilder {
	a.state.IdentityGroups = append(a.state.IdentityGroups, treeStatusAuditAccessGroup)
	return a
}
func (a *FakeAuthBuilder) WithWriteAccess() *FakeAuthBuilder {
	a.state.IdentityGroups = append(a.state.IdentityGroups, treeStatusWriteAccessGroup)
	return a
}
func (a *FakeAuthBuilder) WithPermissionInRealm(permission realms.Permission, realm string) *FakeAuthBuilder {
	a.state.IdentityPermissions = append(a.state.IdentityPermissions, authtest.RealmPermission{
		Permission: permission,
		Realm:      realm,
	})
	return a
}

func (a *FakeAuthBuilder) SetInContext(ctx context.Context) context.Context {
	if a.state.Identity == "anonymous:anonymous" && len(a.state.IdentityGroups) > 0 {
		panic("You cannot call any of the with methods on fakeAuthBuilder if you call the anonymous method")
	}
	return auth.WithState(ctx, a.state)
}
