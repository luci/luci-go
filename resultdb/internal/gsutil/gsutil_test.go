// Copyright 2022 The LUCI Authors.
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

package gsutil

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/storage"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/testutil"
)

func TestGenerateSignedURL(t *testing.T) {
	t.Parallel()

	if os.Getenv("INTEGRATION_TESTS") != "1" {
		t.Skip("talks to real Google Storage")
	}

	ftt.Run(`Generate Signed URL`, t, func(t *ftt.Test) {
		t.Run(`Valid`, func(t *ftt.Test) {
			ctx := auth.WithState(context.Background(), &authtest.FakeState{
				Identity: "user:user@example.com",
			})
			bucket := "testBucket"
			object := "object.txt"
			expiration := clock.Now(ctx).UTC().Add(7 * 24 * time.Hour)
			opts := testutil.GetSignedURLOptions(ctx)

			gsClient, err := storage.NewClient(ctx)
			assert.Loosely(t, err, should.BeNil)

			url, err := GenerateSignedURL(ctx, gsClient, bucket, object, expiration, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, url, should.HavePrefix("https://storage.googleapis.com/testBucket/object.txt?X-Goog-Algorithm=GOOG4-RSA-SHA256&X-Goog-Credential="))
		})
	})
}

func TestSplit(t *testing.T) {
	t.Parallel()

	ftt.Run(`Split`, t, func(t *ftt.Test) {
		t.Run(`Valid`, func(t *ftt.Test) {
			bucket := "testBucket"
			object := "object.txt"
			b, o := Split(fmt.Sprintf("gs://%s/%s", bucket, object))
			assert.Loosely(t, b, should.Equal(bucket))
			assert.Loosely(t, o, should.Equal(object))
		})

		t.Run(`With no object`, func(t *ftt.Test) {
			bucket := "testBucket"
			b, o := Split(fmt.Sprintf("gs://%s", bucket))
			assert.Loosely(t, b, should.Equal(bucket))
			assert.Loosely(t, o, should.BeEmpty)
		})

		t.Run(`With invalid prefix`, func(t *ftt.Test) {
			bucket := "testBucket"
			object := "object.txt"
			path := fmt.Sprintf("xyz://%s/%s", bucket, object)
			b, o := Split(path)
			assert.Loosely(t, b, should.BeEmpty)
			assert.Loosely(t, o, should.Equal(path))
		})
	})
}
