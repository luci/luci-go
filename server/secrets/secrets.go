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
	"errors"
)

var (
	// ErrNoSuchSecret is returned by Store.GetSecret if it can't find a secret.
	ErrNoSuchSecret = errors.New("secret not found")
)

// Secret represents a current value of a secret as well as a set of few
// previous values. Previous values are important when key is being rotated:
// there may be valid outstanding derivatives of previous values of the secret.
type Secret struct {
	Current  []byte   // current value of the secret, always set
	Previous [][]byte // optional list of previous values, most recent first
}

// Blobs returns current blob and all previous blobs as one array.
func (s Secret) Blobs() [][]byte {
	out := make([][]byte, 0, 1+len(s.Previous))
	out = append(out, s.Current)
	out = append(out, s.Previous...)
	return out
}

// Store knows how to retrieve (or autogenerate) a secret given its key.
type Store interface {
	// GetSecret returns a secret given its key.
	//
	// Store may choose to autogenerate a secret if there's no existing one, or it
	// may choose to treat it as a error and return ErrNoSuchSecret.
	GetSecret(name string) (Secret, error)
}

// StaticStore is Store with predefined secrets.
type StaticStore map[string]Secret

// GetSecret returns a secret given its key or ErrNoSuchSecret if no such
// secret exists.
//
// The caller must not mutate the secret.
func (s StaticStore) GetSecret(k string) (Secret, error) {
	if secret, ok := s[k]; ok {
		return secret, nil
	}
	return Secret{}, ErrNoSuchSecret
}
