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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestPagination(t *testing.T) {
	t.Parallel()

	ftt.Run("URLSafeEncrypt/Decrypt", t, func(t *ftt.Test) {
		ctx := GeneratePrimaryTinkAEADForTest(context.Background())

		t.Run("The encrypted string should be URL safe", func(t *ftt.Test) {
			encrypted, err := URLSafeEncrypt(ctx, []byte("sensitive data %=?/&"), []byte("additional data"))
			assert.Loosely(t, err, should.BeNil)
			urlEncoded := url.QueryEscape(encrypted)
			assert.Loosely(t, urlEncoded, should.Equal(encrypted))
		})

		t.Run("Should be able to recover plaintext with the same additional data", func(t *ftt.Test) {
			encrypted, err := URLSafeEncrypt(ctx, []byte("sensitive data"), []byte("additional data"))
			assert.Loosely(t, err, should.BeNil)
			decrypted, err := URLSafeDecrypt(ctx, encrypted, []byte("additional data"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, decrypted, should.Match([]byte("sensitive data")))
		})

		t.Run("Should not be able to recover plaintext with the different additional data", func(t *ftt.Test) {
			encrypted, err := URLSafeEncrypt(ctx, []byte("sensitive data"), []byte("additional data"))
			assert.Loosely(t, err, should.BeNil)
			decrypted, err := URLSafeDecrypt(ctx, encrypted, []byte("additional data 2"))
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, decrypted, should.BeEmpty)
		})
	})
}
