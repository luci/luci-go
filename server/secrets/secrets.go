// Copyright 2015 The LUCI Authors.
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

package secrets

import (
	"bytes"
	"context"
	"errors"
)

var (
	// ErrNoSuchSecret indicates the store can't find the requested secret.
	ErrNoSuchSecret = errors.New("secret not found")
	// ErrNoStoreConfigured indicates there's no Store in the context.
	ErrNoStoreConfigured = errors.New("secrets.Store is not in the context")
)

var contextKey = "secrets.Store"

// Use installs a Store implementation into the context.
func Use(ctx context.Context, s Store) context.Context {
	return context.WithValue(ctx, &contextKey, s)
}

// CurrentStore returns a store installed in the context or nil.
func CurrentStore(ctx context.Context) Store {
	store, _ := ctx.Value(&contextKey).(Store)
	return store
}

// RandomSecret returns a random secret using Store in the context.
//
// If the context doesn't have Store set, returns ErrNoStoreConfigured.
func RandomSecret(ctx context.Context, name string) (Secret, error) {
	if store := CurrentStore(ctx); store != nil {
		return store.RandomSecret(ctx, name)
	}
	return Secret{}, ErrNoStoreConfigured
}

// StoredSecret returns a stored secret using Store in the context.
//
// If the context doesn't have Store set, returns ErrNoStoreConfigured.
func StoredSecret(ctx context.Context, name string) (Secret, error) {
	if store := CurrentStore(ctx); store != nil {
		return store.StoredSecret(ctx, name)
	}
	return Secret{}, ErrNoStoreConfigured
}

// AddRotationHandler registers a callback called when the secret is updated.
//
// If the context doesn't have Store set, returns ErrNoStoreConfigured.
func AddRotationHandler(ctx context.Context, name string, cb RotationHandler) error {
	if store := CurrentStore(ctx); store != nil {
		return store.AddRotationHandler(ctx, name, cb)
	}
	return ErrNoStoreConfigured
}

// Store knows how to retrieve or autogenerate a secret given its name.
//
// See SecretManagerStore for a concrete implementation usually used in
// production.
type Store interface {
	// RandomSecret returns a random secret given its name.
	//
	// The store will auto-generate the secret if necessary. Its value is
	// a random high-entropy blob.
	RandomSecret(ctx context.Context, name string) (Secret, error)

	// StoredSecret returns a previously stored secret given its name.
	//
	// How it was stored depends on the concrete implementation of the Store. The
	// difference from RandomSecret is that the Store will never try to
	// auto-generate such secret if it is missing and will return ErrNoSuchSecret
	// instead.
	StoredSecret(ctx context.Context, name string) (Secret, error)

	// AddRotationHandler registers a callback called when the secret is updated.
	//
	// Useful when a value of StoredSecret(...) is used to derive something else.
	// The callback allows the store to notify the consumer of the secret when
	// it changes.
	AddRotationHandler(ctx context.Context, name string, cb RotationHandler) error
}

// RotationHandler is called from an internal goroutine after the store fetches
// a new version of a stored secret.
type RotationHandler func(context.Context, Secret)

// Secret represents multiple versions of some secret blob.
//
// There's a current version (which is always set) that should be used for all
// kinds of operations: active (like encryption, signing, etc) and passive
// (like decryption, checking signatures, etc).
//
// And there's zero or more other versions that should be used only for passive
// operations. Other versions contain previous or future values of the secret.
// They are important for implementing graceful rotation of the secret.
type Secret struct {
	Active  []byte   // current value of the secret, always set
	Passive [][]byte // optional list of other values, in no particular order
}

// Blobs returns the active version and all passive versions as one array.
func (s Secret) Blobs() [][]byte {
	out := make([][]byte, 0, 1+len(s.Passive))
	out = append(out, s.Active)
	out = append(out, s.Passive...)
	return out
}

// Equal returns true if secrets are equal.
//
// Does *not* run in constant time. Shouldn't be used in a cryptographic
// context due to susceptibility to timing attacks.
func (s Secret) Equal(a Secret) bool {
	switch {
	case len(s.Passive) != len(a.Passive):
		return false
	case !bytes.Equal(s.Active, a.Active):
		return false
	}
	for i, blob := range s.Passive {
		if !bytes.Equal(blob, a.Passive[i]) {
			return false
		}
	}
	return true
}
