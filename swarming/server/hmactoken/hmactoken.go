// Copyright 2023 The LUCI Authors.
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

// Package hmactoken implements generation and validation HMAC-tagged Swarming
// tokens.
package hmactoken

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"sync/atomic"

	"go.chromium.org/luci/server/secrets"
)

// Secret can be used to generate and validate HMAC-tagged tokens.
type Secret struct {
	hmacSecret atomic.Value // stores secrets.Secret
}

// NewRotatingSecret creates a new secret given a key name and subscribes to its
// rotations.
func NewRotatingSecret(ctx context.Context, keyName string) (*Secret, error) {
	s := &Secret{}

	// Load the initial value of the key used to HMAC-tag tokens.
	key, err := secrets.StoredSecret(ctx, keyName)
	if err != nil {
		return nil, err
	}
	s.hmacSecret.Store(key)

	// Update the cached value whenever the secret rotates.
	err = secrets.AddRotationHandler(ctx, keyName, func(_ context.Context, key secrets.Secret) {
		s.hmacSecret.Store(key)
	})
	if err != nil {
		return nil, err
	}

	return s, nil
}

// NewStaticSecret creates a new secret from a given static secret value.
//
// Mostly for tests.
func NewStaticSecret(secret secrets.Secret) *Secret {
	s := &Secret{}
	s.hmacSecret.Store(secret)
	return s
}

// Tag generates a HMAC-SHA256 tag of `pfx+body`.
func (s *Secret) Tag(pfx, body []byte) []byte {
	secret := s.hmacSecret.Load().(secrets.Secret).Active
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(pfx)
	_, _ = mac.Write(body)
	return mac.Sum(nil)
}

// Verify checks the given tag matches the expected one for given `pfx+body`.
func (s *Secret) Verify(pfx, body, tag []byte) bool {
	secret := s.hmacSecret.Load().(secrets.Secret)
	for _, key := range secret.Blobs() {
		mac := hmac.New(sha256.New, key)
		_, _ = mac.Write(pfx)
		_, _ = mac.Write(body)
		if hmac.Equal(mac.Sum(nil), tag) {
			return true
		}
	}
	return false
}
