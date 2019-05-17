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
	"crypto/sha256"
	"io"
	"sync"

	"golang.org/x/crypto/hkdf"
)

// DerivedStore implements Store by deriving secrets from some single master
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

// GetSecret returns a generated secret given its key.
func (d *DerivedStore) GetSecret(name string) (Secret, error) {
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
	s := Secret{Current: derive(d.root.Current, name)}
	if len(d.root.Previous) != 0 {
		s.Previous = make([][]byte, len(d.root.Previous))
		for i, secret := range d.root.Previous {
			s.Previous[i] = derive(secret, name)
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
