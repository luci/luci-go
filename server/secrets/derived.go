// Copyright 2019 The LUCI Authors.
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
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"sync"

	"golang.org/x/crypto/hkdf"
)

// DerivedStore implements Store by deriving secrets from some single root
// secret using HKDF.
//
// Caches all derived secrets internally forever. Assumes the set of possible
// key names is limited.
type DerivedStore struct {
	m     sync.RWMutex
	root  Secret
	cache map[string]Secret
}

// NewDerivedStore returns a store that derives secrets from the given root key.
func NewDerivedStore(root Secret) *DerivedStore {
	return &DerivedStore{
		root:  root,
		cache: map[string]Secret{},
	}
}

// RandomSecret returns a generated secret given its name.
func (d *DerivedStore) RandomSecret(ctx context.Context, name string) (Secret, error) {
	d.m.RLock()
	s, ok := d.cache[name]
	d.m.RUnlock()
	if ok {
		return s, nil
	}

	d.m.Lock()
	if s, ok = d.cache[name]; !ok {
		s = d.generateLocked(name)
		d.cache[name] = s
	}
	d.m.Unlock()

	return s, nil
}

// StoredSecret returns an error, since DerivedStore always derives secrets.
func (d *DerivedStore) StoredSecret(ctx context.Context, name string) (Secret, error) {
	return Secret{}, errors.New("DerivedStore: stored secrets are not supported")
}

// AddRotationHandler is not implemented.
func (d *DerivedStore) AddRotationHandler(ctx context.Context, name string, cb RotationHandler) error {
	return errors.New("not implemented")
}

// SetRoot replaces the root key used to derive secrets.
func (d *DerivedStore) SetRoot(root Secret) {
	d.m.RLock()
	same := d.root.Equal(root)
	d.m.RUnlock()
	if !same {
		d.m.Lock()
		d.root = root
		d.cache = map[string]Secret{}
		d.m.Unlock()
	}
}

func (d *DerivedStore) generateLocked(name string) Secret {
	s := Secret{Active: derive(d.root.Active, name)}
	if len(d.root.Passive) != 0 {
		s.Passive = make([][]byte, len(d.root.Passive))
		for i, secret := range d.root.Passive {
			s.Passive[i] = derive(secret, name)
		}
	}
	return s
}

func derive(secret []byte, name string) []byte {
	// Note: we don't use salt (nil) because we want the derivation process to be
	// deterministic.
	hkdf := hkdf.New(sha256.New, secret, nil, []byte(name))
	key := make([]byte, 16)
	if _, err := io.ReadFull(hkdf, key); err != nil {
		panic(err)
	}
	return key
}
