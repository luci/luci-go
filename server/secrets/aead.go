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

package secrets

import (
	"bytes"
	"context"
	"sync/atomic"
	"time"

	"github.com/google/tink/go/aead"
	"github.com/google/tink/go/insecurecleartextkeyset"
	"github.com/google/tink/go/keyset"
	"github.com/google/tink/go/tink"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/caching"
)

// A never-shrinking, never-expiring cache of loaded *AEADHandle.
//
// It is essentially a map (living in the process memory cache) with some
// synchronization around item instantiation.
var handles = caching.RegisterLRUCache(0)

// AEADHandle implements tink.AEAD by delegating to an atomically updated
// tink.AEAD primitive stored inside.
//
// The value stored inside is updated whenever the underlying secret with
// the keyset is rotated. This can happen at any time, even between two
// consecutive Encrypt calls. Use Unwrap to get an immutable copy of the
// current tink.AEAD primitive.
type AEADHandle struct {
	val atomic.Value
}

// Unwrap returns the current immutable tink.AEAD pointed to by the handle.
//
// Useful if you depend on Encrypt/Decrypt operations to not spontaneously
// change keys or you are calling them in a tight loop.
//
// Do not retain the returned tink.AEAD for long. It will become essentially
// incorrect when the underlying keyset is rotated.
func (h *AEADHandle) Unwrap() tink.AEAD {
	return h.val.Load().(tink.AEAD)
}

// Encrypt is part of tink.AEAD interface.
func (h *AEADHandle) Encrypt(plaintext, additionalData []byte) ([]byte, error) {
	return h.Unwrap().Encrypt(plaintext, additionalData)
}

// Decrypt is part of tink.AEAD interface.
func (h *AEADHandle) Decrypt(ciphertext, additionalData []byte) ([]byte, error) {
	return h.Unwrap().Decrypt(ciphertext, additionalData)
}

// LoadTinkAEAD loads a tink AEAD key from the given secret, subscribing to
// its rotation.
//
// Returns a handle that points to a tink.AEAD primitive updated atomically when
// the underlying keys are rotated. You can either use the handle directly as a
// tink.AEAD primitive itself (in which case its underlying keyset may be
// changing between individual Encrypt/Decrypt operations), or get the current
// immutable tink.AEAD primitive via Unwrap() method. The latter is useful if
// you depend on Encrypt/Decrypt operations to not spontaneously change keys or
// you are calling them in a tight loop.
//
// If the context has a process cache initialized (true for contexts in
// production code), loaded keys are cached there, i.e. calling LoadTinkAEAD
// twice with the same `secretName` value will return the exact same object.
// If the context doesn't have a process cache (happens in tests), LoadTinkAEAD
// constructs a new handle each time it is called.
//
// The returned AEADHandle is always valid. If a new value of the rotated secret
// is malformed, the handle will retain its old keyset. Once loaded, it never
// "spoils".
func LoadTinkAEAD(ctx context.Context, secretName string) (*AEADHandle, error) {
	cache := handles.LRU(ctx)
	if cache == nil {
		// We don't have a process cache, this is likely a test. Do not subscribe
		// to rotations: if LoadTinkAEAD is called often, we'll be adding more and
		// more subscription handlers, essentially leaking memory.
		return loadTinkAEADLocked(ctx, secretName, false)
	}
	handle, err := cache.GetOrCreate(ctx, secretName, func() (interface{}, time.Duration, error) {
		handle, err := loadTinkAEADLocked(ctx, secretName, true)
		return handle, 0, err
	})
	if err != nil {
		return nil, err
	}
	return handle.(*AEADHandle), nil
}

func loadTinkAEADLocked(ctx context.Context, secretName string, subscribe bool) (*AEADHandle, error) {
	secret, err := StoredSecret(ctx, secretName)
	if err != nil {
		return nil, errors.Annotate(err, "failed to load Tink AEAD key %q", secretName).Err()
	}

	aead, err := deserializeKeyset(&secret)
	if err != nil {
		return nil, errors.Annotate(err, "failed to deserialize Tink AEAD key %q", secretName).Err()
	}

	handle := &AEADHandle{}
	handle.val.Store(aead)

	if subscribe {
		err := AddRotationHandler(ctx, secretName, func(ctx context.Context, secret Secret) {
			aead, err := deserializeKeyset(&secret)
			if err != nil {
				logging.Errorf(ctx, "Rotated Tink AEAD key %q is broken, ignoring it: %s", secretName, err)
			} else {
				logging.Infof(ctx, "Tink AEAD key %q was rotated", secretName)
				handle.val.Store(aead)
			}
		})
		if err != nil {
			return nil, errors.Annotate(err, "failed to subscribe to the Tink AEAD key %q rotation", secretName).Err()
		}
	}

	return handle, nil
}

func deserializeKeyset(s *Secret) (tink.AEAD, error) {
	kh, err := insecurecleartextkeyset.Read(keyset.NewJSONReader(bytes.NewReader(s.Current)))
	if err != nil {
		return nil, err
	}
	return aead.New(kh)
}
