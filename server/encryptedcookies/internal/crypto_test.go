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

package internal

import (
	"bytes"
	"testing"

	"github.com/google/tink/go/aead"
	"github.com/google/tink/go/keyset"

	"go.chromium.org/luci/server/encryptedcookies/internal/encryptedcookiespb"
	"go.chromium.org/luci/server/encryptedcookies/session/sessionpb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGenerateNonce(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		nonce1 := GenerateNonce()
		nonce2 := GenerateNonce()
		So(nonce1, ShouldHaveLength, 16)
		So(bytes.Equal(nonce1, nonce2), ShouldBeFalse)
	})
}

func TestCrypto(t *testing.T) {
	t.Parallel()

	Convey("With keyset", t, func() {
		kh, err := keyset.NewHandle(aead.AES256GCMKeyTemplate())
		So(err, ShouldBeNil)
		ae, err := aead.New(kh)
		So(err, ShouldBeNil)

		Convey("State enc/dec", func() {
			state := &encryptedcookiespb.OpenIDState{DestHost: "blah"}

			enc, err := EncryptStateB64(ae, state)
			So(err, ShouldBeNil)

			dec, err := DecryptStateB64(ae, enc)
			So(err, ShouldBeNil)
			So(dec, ShouldResembleProto, state)

			_, err = DecryptStateB64(ae, "aaaaaaaa"+enc[8:])
			So(err, ShouldNotBeNil)
		})

		Convey("Private enc/dec", func() {
			priv := &sessionpb.Private{AccessToken: "blah"}

			enc, err := EncryptPrivate(ae, priv)
			So(err, ShouldBeNil)

			dec, err := DecryptPrivate(ae, enc)
			So(err, ShouldBeNil)
			So(dec, ShouldResembleProto, priv)

			for i := 0; i < 8; i++ {
				enc[i] = 0
			}
			_, err = DecryptPrivate(ae, enc)
			So(err, ShouldNotBeNil)
		})
	})
}
