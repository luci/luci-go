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
	"context"
	"net/url"
	"testing"

	"github.com/google/tink/go/aead"
	"github.com/google/tink/go/keyset"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPagination(t *testing.T) {
	t.Parallel()

	Convey("URLSafeEncrypt/Decrypt", t, func() {
		ctx := context.Background()

		kh, err := keyset.NewHandle(aead.AES256GCMKeyTemplate())
		So(err, ShouldBeNil)
		aead, err := aead.New(kh)
		So(err, ShouldBeNil)
		ctx = SetPrimaryTinkAEADForTest(ctx, aead)

		Convey("The encrypted string should be URL safe", func() {
			encrypted, err := URLSafeEncrypt(ctx, []byte("sensitive data %=?/&"), []byte("additional data"))
			So(err, ShouldBeNil)
			urlEncoded := url.QueryEscape(encrypted)
			So(urlEncoded, ShouldEqual, encrypted)
		})

		Convey("Should be able to recover plaintext with the same additional data", func() {
			encrypted, err := URLSafeEncrypt(ctx, []byte("sensitive data"), []byte("additional data"))
			So(err, ShouldBeNil)
			decrypted, err := URLSafeDecrypt(ctx, encrypted, []byte("additional data"))
			So(err, ShouldBeNil)
			So(decrypted, ShouldResemble, []byte("sensitive data"))
		})

		Convey("Should not be able to recover plaintext with the different additional data", func() {
			encrypted, err := URLSafeEncrypt(ctx, []byte("sensitive data"), []byte("additional data"))
			So(err, ShouldBeNil)
			decrypted, err := URLSafeDecrypt(ctx, encrypted, []byte("additional data 2"))
			So(err, ShouldNotBeNil)
			So(decrypted, ShouldBeEmpty)
		})
	})
}
