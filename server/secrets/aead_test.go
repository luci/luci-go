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

	"go.chromium.org/luci/server/caching"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestAEAD(t *testing.T) {
	t.Parallel()

	Convey("With context", t, func() {
		const secretName = "some-secret-name"

		// Prep a valid keyset.
		kh, err := keyset.NewHandle(aead.AES256GCMKeyTemplate())
		So(err, ShouldBeNil)
		buf := &bytes.Buffer{}
		So(insecurecleartextkeyset.Write(kh, keyset.NewJSONWriter(buf)), ShouldBeNil)

		goodKey := buf.Bytes()
		badKey := []byte("this is not a valid keyset")

		store := &fakeStore{expectedName: secretName}
		ctx := Use(context.Background(), store)

		putSecret := func(val []byte) {
			store.secret = Secret{Current: val}
		}

		Convey("PrimaryTinkAEADKey works", func() {
			So(PrimaryTinkAEAD(ctx), ShouldBeNil)
			_, err := Encrypt(ctx, []byte("ignored"), []byte("ignored"))
			So(err, ShouldEqual, ErrNoPrimaryAEAD)
			_, err = Decrypt(ctx, []byte("ignored"), []byte("ignored"))
			So(err, ShouldEqual, ErrNoPrimaryAEAD)

			ctx = GeneratePrimaryTinkAEADForTest(ctx)
			So(PrimaryTinkAEAD(ctx).Unwrap(), ShouldNotBeNil)

			ciphertext, err := Encrypt(ctx,
				[]byte("secret-to-be-encrypted"),
				[]byte("authenticated-only-must-match-in-decrypt"),
			)
			So(err, ShouldBeNil)

			cleartext, err := Decrypt(ctx,
				ciphertext,
				[]byte("authenticated-only-must-match-in-decrypt"),
			)
			So(err, ShouldBeNil)
			So(string(cleartext), ShouldEqual, "secret-to-be-encrypted")
		})

		Convey("Without process cache", func() {
			Convey("Good keyset", func() {
				putSecret(goodKey)

				k1, err := LoadTinkAEAD(ctx, secretName)
				So(err, ShouldBeNil)
				So(k1, ShouldNotBeNil)

				k2, err := LoadTinkAEAD(ctx, secretName)
				So(err, ShouldBeNil)
				So(k2, ShouldNotBeNil)

				// Got different objects.
				So(k1, ShouldNotEqual, k2)
				So(k1.Unwrap(), ShouldNotEqual, k2.Unwrap())
			})

			Convey("Bad keyset", func() {
				putSecret(badKey)
				_, err := LoadTinkAEAD(ctx, secretName)
				So(err, ShouldErrLike, "failed to deserialize")
			})
		})

		Convey("With process cache", func() {
			ctx := caching.WithEmptyProcessCache(ctx)

			Convey("Caching", func() {
				putSecret(goodKey)

				k1, err := LoadTinkAEAD(ctx, secretName)
				So(err, ShouldBeNil)
				So(k1, ShouldNotBeNil)

				k2, err := LoadTinkAEAD(ctx, secretName)
				So(err, ShouldBeNil)
				So(k2, ShouldNotBeNil)

				// Got the exact same object.
				So(k1, ShouldEqual, k2)
			})

			Convey("Rotation", func() {
				putSecret(goodKey)

				k, err := LoadTinkAEAD(ctx, secretName)
				So(err, ShouldBeNil)
				So(k, ShouldNotBeNil)

				// Remember the tink.AEAD pre rotation.
				pre := k.Unwrap()

				Convey("Good new key", func() {
					// Call the handler with a valid secret value (reuse the current one,
					// this is not essential for the test).
					putSecret(goodKey)
					store.handler(ctx, store.secret)
					// Got a different tink.AEAD now.
					So(k.Unwrap(), ShouldNotEqual, pre)
				})

				Convey("Bad new key", func() {
					// Call the handler with a bad secret value.
					putSecret(badKey)
					store.handler(ctx, store.secret)
					// tink.AEAD is unchanged.
					So(k.Unwrap(), ShouldEqual, pre)
				})
			})
		})
	})
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
