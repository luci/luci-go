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
	// ErrNoSuchSecret is returned by GetSecret if it can't find a secret.
	ErrNoSuchSecret = errors.New("secret not found")
	// ErrNoStoreConfigured is returned by GetSecret if the secret store is not in
	// the context.
	ErrNoStoreConfigured = errors.New("secrets.Store is not in the context")
)

var contextKey = "secrets.Store"

// Use installs a Store implementation into the context.
func Use(ctx context.Context, s Store) context.Context {
	return context.WithValue(ctx, &contextKey, s)
}

// GetSecret returns a secret using Store in the context.
//
// If the context doesn't have Store set, returns ErrNoStoreConfigured.
func GetSecret(ctx context.Context, key string) (Secret, error) {
	if store, _ := ctx.Value(&contextKey).(Store); store != nil {
		return store.GetSecret(ctx, key)
	}
	return Secret{}, ErrNoStoreConfigured
}

// Store knows how to retrieve or autogenerate a secret given its key.
type Store interface {
	// GetSecret returns a secret given its key.
	//
	// Store may choose to autogenerate a secret if there's no existing one, or it
	// may choose to treat it as an error and return ErrNoSuchSecret.
	GetSecret(ctx context.Context, name string) (Secret, error)
}

// Secret represents a current value of a secret as well as a set of few
// previous values. Previous values are important when the secret is being
// rotated: there may be valid outstanding derivatives of previous values of
// the secret.
type Secret struct {
	Current  []byte   `json:"current"`            // current value of the secret, always set
	Previous [][]byte `json:"previous,omitempty"` // optional list of previous values, most recent first
}

// Blobs returns current blob and all previous blobs as one array.
func (s Secret) Blobs() [][]byte {
	out := make([][]byte, 0, 1+len(s.Previous))
	out = append(out, s.Current)
	out = append(out, s.Previous...)
	return out
}

// Equal returns true if secrets are equal.
//
// Does *not* run in constant time. Shouldn't be used in a cryptographic
// context due to susceptibility to timing attacks.
func (s Secret) Equal(a Secret) bool {
	switch {
	case len(s.Previous) != len(a.Previous):
		return false
	case !bytes.Equal(s.Current, a.Current):
		return false
	}
	for i, blob := range s.Previous {
		if !bytes.Equal(blob, a.Previous[i]) {
			return false
		}
	}
	return true
}
