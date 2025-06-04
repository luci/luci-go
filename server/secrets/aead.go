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
	tinkpb "github.com/google/tink/go/proto/tink_go_proto"
	"github.com/google/tink/go/tink"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server/caching"
)

// ErrNoPrimaryAEAD indicates the context doesn't have a primary AEAD primitive
// installed.
//
// For production code it usually happens if `-primary-tink-aead-key` flag
// wasn't set.
//
// For test code, it happens if the test context wasn't prepared correctly. See
// GeneratePrimaryTinkAEADForTest for generating a random key for tests.
var ErrNoPrimaryAEAD = errors.New("the primary AEAD primitive is not configured")

// A never-shrinking, never-expiring cache of loaded *AEADHandle.
//
// It is essentially a map (living in the process memory cache) with some
// synchronization around item instantiation.
var handles = caching.RegisterLRUCache[string, *AEADHandle](0)

// primaryAEADCtxKey is used to construct context key for PrimaryTinkAEAD.
var primaryAEADCtxKey = "go.chromium.org/luci/secrets.PrimaryTinkAEAD"

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

// PrimaryTinkAEAD returns a handle pointing to an AEAD primitive to use by
// default in the process.
//
// See https://pkg.go.dev/github.com/google/tink/go/tink#AEAD for the
// description of the AEAD primitive.
//
// Make sure to append a context string to `additionalData` when calling
// Encrypt/Decrypt to guarantee that a cipher text generated in one context
// isn't unexpectedly reused in another context. Failure to do so can lead to
// compromises.
//
// In production the keyset behind PrimaryTinkAEAD is specified via
// the `-primary-tink-aead-key` flag. This flag is optional. PrimaryTinkAEAD
// will return nil if the `-primary-tink-aead-key` flag was omitted. Code that
// depends on a presence of an AEAD implementation must check that the return
// value of PrimaryTinkAEAD is not nil during startup.
//
// Tests can use GeneratePrimaryTinkAEADForTest to prepare a context with some
// randomly generated key.
func PrimaryTinkAEAD(ctx context.Context) *AEADHandle {
	val, _ := ctx.Value(&primaryAEADCtxKey).(*AEADHandle)
	return val
}

// GeneratePrimaryTinkAEADForTest generates a new key and sets it as primary.
//
// Must be used only in tests.
func GeneratePrimaryTinkAEADForTest(ctx context.Context) context.Context {
	kh, err := keyset.NewHandle(aead.AES256GCMKeyTemplate())
	if err != nil {
		panic(err)
	}
	aead, err := aead.New(kh)
	if err != nil {
		panic(err)
	}
	handle := &AEADHandle{}
	handle.val.Store(aead)
	return setPrimaryTinkAEAD(ctx, handle)
}

// Encrypt encrypts `plaintext` with `additionalData` as additional
// authenticated data using the primary tink AEAD primitive in the context.
//
// See PrimaryTinkAEAD for caveats.
//
// Returns ErrNoPrimaryAEAD if there's no primary AEAD primitive in the context.
func Encrypt(ctx context.Context, plaintext, additionalData []byte) ([]byte, error) {
	if aead := PrimaryTinkAEAD(ctx); aead != nil {
		return aead.Encrypt(plaintext, additionalData)
	}
	return nil, ErrNoPrimaryAEAD
}

// Decrypt decrypts `ciphertext` with `additionalData` as additional
// authenticated data using the primary tink AEAD primitive in the context.
//
// See PrimaryTinkAEAD for caveats.
//
// Returns ErrNoPrimaryAEAD if there's no primary AEAD primitive in the context.
func Decrypt(ctx context.Context, ciphertext, additionalData []byte) ([]byte, error) {
	if aead := PrimaryTinkAEAD(ctx); aead != nil {
		return aead.Decrypt(ciphertext, additionalData)
	}
	return nil, ErrNoPrimaryAEAD
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
	return cache.GetOrCreate(ctx, secretName, func() (*AEADHandle, time.Duration, error) {
		handle, err := loadTinkAEADLocked(ctx, secretName, true)
		return handle, 0, err
	})
}

func setPrimaryTinkAEAD(ctx context.Context, val *AEADHandle) context.Context {
	return context.WithValue(ctx, &primaryAEADCtxKey, val)
}

func loadTinkAEADLocked(ctx context.Context, secretName string, subscribe bool) (*AEADHandle, error) {
	secret, err := StoredSecret(ctx, secretName)
	if err != nil {
		return nil, errors.Fmt("failed to load Tink AEAD key %q: %w", secretName, err)
	}

	aead, info, err := mergedKeyset(&secret)
	if err != nil {
		return nil, errors.Fmt("failed to deserialize Tink AEAD key %q: %w", secretName, err)
	}
	logging.Infof(ctx, "Loaded Tink AEAD key %q (primary Tink key is %d)", secretName, info.PrimaryKeyId)

	handle := &AEADHandle{}
	handle.val.Store(aead)

	if subscribe {
		err := AddRotationHandler(ctx, secretName, func(ctx context.Context, secret Secret) {
			aead, info, err := mergedKeyset(&secret)
			if err != nil {
				logging.Errorf(ctx, "Rotated Tink AEAD key %q is broken, ignoring it: %s", secretName, err)
			} else {
				logging.Infof(ctx, "Tink AEAD key %q was rotated (primary Tink key is %d)", secretName, info.PrimaryKeyId)
				handle.val.Store(aead)
			}
		})
		if err != nil {
			return nil, errors.Fmt("failed to subscribe to the Tink AEAD key %q rotation: %w", secretName, err)
		}
	}

	return handle, nil
}

// mergedKeyset takes a secret with multiple versions of a JSON-serialized
// unencrypted Tink keyset and builds a merged keyset out of it, and constructs
// an AEAD primitive that uses all keys there. Additionally returns information
// about the final keyset.
//
// Ignores disabled or deleted keys.
func mergedKeyset(s *Secret) (tink.AEAD, *tinkpb.KeysetInfo, error) {
	merged := &tinkpb.Keyset{}
	seenKeyIDs := map[uint32]bool{}

	// Adds keys from a serialized keyset to `merged`, updating its primary key.
	appendKeySet := func(blob []byte) error {
		ks, err := keyset.NewJSONReader(bytes.NewReader(blob)).Read()
		if err != nil {
			return errors.Fmt("failed to deserialize Tink keyset: %w", err)
		}
		for _, key := range ks.Key {
			if key.Status == tinkpb.KeyStatusType_ENABLED && !seenKeyIDs[key.KeyId] {
				seenKeyIDs[key.KeyId] = true
				merged.Key = append(merged.Key, key)
			}
		}
		if !seenKeyIDs[ks.PrimaryKeyId] {
			return errors.Fmt("keyset references unknown key %d as primary", ks.PrimaryKeyId)
		}
		merged.PrimaryKeyId = ks.PrimaryKeyId
		return nil
	}

	// Visit all keysets. Visit the active last, to make sure we pick up its key
	// as the final primary.
	for idx, blob := range s.Passive {
		if err := appendKeySet(blob); err != nil {
			return nil, nil, errors.Fmt("passive keyset #%d: %w", idx+1, err)
		}
	}
	if err := appendKeySet(s.Active); err != nil {
		return nil, nil, errors.Fmt("active keyset: %w", err)
	}

	// Build an AEAD primitive out of the merged keyset.
	kh, err := insecurecleartextkeyset.Read(&keyset.MemReaderWriter{
		Keyset: merged,
	})
	if err != nil {
		return nil, nil, err
	}
	aead, err := aead.New(kh)
	if err != nil {
		return nil, nil, err
	}
	return aead, kh.KeysetInfo(), nil
}
