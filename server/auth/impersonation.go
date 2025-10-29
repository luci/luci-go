// Copyright 2025 The LUCI Authors.
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

package auth

import (
	"context"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

// UncheckedImpersonationMethod is a placeholder Method that is placed in
// the auth state when using unchecked impersonation.
//
// It exists just to make sure the state produced by WithUncheckedImpersonation
// conforms to the expected interface (and doesn't propagate the original
// authenticator that has nothing to do with the impersonated state).
type UncheckedImpersonationMethod struct{}

// Authenticate implements Method.
func (UncheckedImpersonationMethod) Authenticate(context.Context, RequestMetadata) (*User, Session, error) {
	return nil, nil, errors.Fmt("attempting to authenticate via UncheckedImpersonationMethod: this is wrong, it doesn't do authentication")
}

var uncheckedImpersonationMethod Method = UncheckedImpersonationMethod{}

// WithUncheckedImpersonation changes the state in the context such that
// CurrentIdentity(...) and similar methods will start using the given identity
// (and in particular all HasPermission checks done over the return context will
// use this identity as well).
//
// It is assumed the caller of this function has already verified in some way
// that such impersonation is safe. No permissions are checked (which is what
// "unchecked" in the name conveys).
//
// The current state in the context must identify a non-anonymous, not already
// impersonated, identity. This identity will be placed into the PeerIdentity()
// of the new auth state.
//
// This is a dangerous function, avoid if possible.
func WithUncheckedImpersonation(ctx context.Context, ident identity.Identity) (context.Context, error) {
	st := GetState(ctx)
	if st == nil {
		return nil, errors.Fmt("%w: no auth state in the context", ErrIncorrectImpersonation)
	}

	cur := st.User().Identity
	if cur == identity.AnonymousIdentity {
		return nil, errors.Fmt("%w: an anonymous can't use impersonation", ErrIncorrectImpersonation)
	}
	if cur != st.PeerIdentity() {
		return nil, errors.Fmt("%w: impersonation is already in effect, can't chain it", ErrIncorrectImpersonation)
	}

	logging.Infof(ctx, "Impersonation is used: %q is pretending to be %q", cur, ident)
	return WithState(ctx, &state{
		authenticator: &Authenticator{
			Methods: []Method{uncheckedImpersonationMethod},
		},
		db:     st.DB(),
		method: uncheckedImpersonationMethod,
		user: &User{
			Identity: ident,
			Email:    ident.Email(),
		},
		peerIdent:  st.PeerIdentity(),
		peerIP:     st.PeerIP(),
		endUserErr: ErrNoForwardableCreds,
	}), nil
}
