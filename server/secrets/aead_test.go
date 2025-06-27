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
	"testing"

	"github.com/google/tink/go/aead"
	"github.com/google/tink/go/insecurecleartextkeyset"
	"github.com/google/tink/go/keyset"
	tinkpb "github.com/google/tink/go/proto/tink_go_proto"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/caching"
)

func TestAEAD(t *testing.T) {
	t.Parallel()

	ftt.Run("With context", t, func(t *ftt.Test) {
		const secretName = "some-secret-name"

		goodKey, _ := genTestKeySet(2)
		badKey := []byte("this is not a valid keyset")

		store := &fakeStore{expectedName: secretName}
		ctx := Use(context.Background(), store)

		putSecret := func(val []byte) {
			store.secret = Secret{Active: val}
		}

		t.Run("PrimaryTinkAEADKey works", func(t *ftt.Test) {
			assert.Loosely(t, PrimaryTinkAEAD(ctx), should.BeNil)
			_, err := Encrypt(ctx, []byte("ignored"), []byte("ignored"))
			assert.Loosely(t, err, should.Equal(ErrNoPrimaryAEAD))
			_, err = Decrypt(ctx, []byte("ignored"), []byte("ignored"))
			assert.Loosely(t, err, should.Equal(ErrNoPrimaryAEAD))

			ctx = GeneratePrimaryTinkAEADForTest(ctx)
			assert.Loosely(t, PrimaryTinkAEAD(ctx).Unwrap(), should.NotBeNil)

			ciphertext, err := Encrypt(ctx,
				[]byte("secret-to-be-encrypted"),
				[]byte("authenticated-only-must-match-in-decrypt"),
			)
			assert.Loosely(t, err, should.BeNil)

			cleartext, err := Decrypt(ctx,
				ciphertext,
				[]byte("authenticated-only-must-match-in-decrypt"),
			)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(cleartext), should.Equal("secret-to-be-encrypted"))
		})

		t.Run("Without process cache", func(t *ftt.Test) {
			t.Run("Good keyset", func(t *ftt.Test) {
				putSecret(goodKey)

				k1, err := LoadTinkAEAD(ctx, secretName)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, k1, should.NotBeNil)

				k2, err := LoadTinkAEAD(ctx, secretName)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, k2, should.NotBeNil)

				// Got different objects.
				assert.Loosely(t, k1, should.NotEqual(k2))
				assert.Loosely(t, k1.Unwrap(), should.NotEqual(k2.Unwrap()))
			})

			t.Run("Bad keyset", func(t *ftt.Test) {
				putSecret(badKey)
				_, err := LoadTinkAEAD(ctx, secretName)
				assert.Loosely(t, err, should.ErrLike("failed to deserialize"))
			})
		})

		t.Run("With process cache", func(t *ftt.Test) {
			ctx := caching.WithEmptyProcessCache(ctx)

			t.Run("Caching", func(t *ftt.Test) {
				putSecret(goodKey)

				k1, err := LoadTinkAEAD(ctx, secretName)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, k1, should.NotBeNil)

				k2, err := LoadTinkAEAD(ctx, secretName)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, k2, should.NotBeNil)

				// Got the exact same object.
				assert.Loosely(t, k1, should.Equal(k2))
			})

			t.Run("Rotation", func(t *ftt.Test) {
				putSecret(goodKey)

				k, err := LoadTinkAEAD(ctx, secretName)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, k, should.NotBeNil)

				// Remember the tink.AEAD pre rotation.
				pre := k.Unwrap()

				t.Run("Good new key", func(t *ftt.Test) {
					// Call the handler with a valid secret value (reuse the current one,
					// this is not essential for the test).
					putSecret(goodKey)
					store.handler(ctx, store.secret)
					// Got a different tink.AEAD now.
					assert.Loosely(t, k.Unwrap(), should.NotEqual(pre))
				})

				t.Run("Bad new key", func(t *ftt.Test) {
					// Call the handler with a bad secret value.
					putSecret(badKey)
					store.handler(ctx, store.secret)
					// tink.AEAD is unchanged.
					assert.Loosely(t, k.Unwrap(), should.Equal(pre))
				})
			})
		})
	})
}

func TestMergedKeyset(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		ks1, info1 := genTestKeySet(2)
		ks2, info2 := genTestKeySet(3)
		ks3, info3 := genTestKeySet(1)

		merged, mergedInfo, err := mergedKeyset(&Secret{
			Active: ks3,
			Passive: [][]byte{
				ks1,
				ks2,
				ks1, // will be ignored
			},
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, merged, should.NotBeNil)

		assert.Loosely(t, mergedInfo.PrimaryKeyId, should.Equal(info3.PrimaryKeyId))
		assert.Loosely(t, mergedInfo.KeyInfo, should.Match([]*tinkpb.KeysetInfo_KeyInfo{
			info1.KeyInfo[0],
			info1.KeyInfo[1],
			info2.KeyInfo[0],
			info2.KeyInfo[1],
			info2.KeyInfo[2],
			info3.KeyInfo[0],
		}))
	})
}

func genTestKeySet(keys int) ([]byte, *tinkpb.KeysetInfo) {
	ksm := keyset.NewManager()
	for range keys {
		keyID, err := ksm.Add(aead.AES256GCMKeyTemplate())
		if err != nil {
			panic(err)
		}
		if err := ksm.SetPrimary(keyID); err != nil {
			panic(err)
		}
	}
	kh, err := ksm.Handle()
	if err != nil {
		panic(err)
	}
	buf := &bytes.Buffer{}
	if err := insecurecleartextkeyset.Write(kh, keyset.NewJSONWriter(buf)); err != nil {
		panic(err)
	}
	return buf.Bytes(), kh.KeysetInfo()
}

type fakeStore struct {
	expectedName string
	secret       Secret
	handler      RotationHandler
}

func (s *fakeStore) RandomSecret(ctx context.Context, name string) (Secret, error) {
	panic("unused in this test")
}

func (s *fakeStore) StoredSecret(ctx context.Context, name string) (Secret, error) {
	if name != s.expectedName {
		panic("unexpected secret name: " + name)
	}
	return s.secret, nil
}

func (s *fakeStore) AddRotationHandler(ctx context.Context, name string, cb RotationHandler) error {
	if s.handler != nil {
		panic("AddRotationHandler called more than once")
	}
	s.handler = cb
	return nil
}
