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
	"testing"

	"cloud.google.com/go/storage"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"time"

	"go.chromium.org/luci/resultdb/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGenerateSignedURL(t *testing.T) {
	t.Parallel()

	Convey(`Generate Signed URL`, t, func() {
		Convey(`Valid`, func() {
			ctx := auth.WithState(context.Background(), &authtest.FakeState{
				Identity: "user:user@example.com",
			})
			bucket := "testBucket"
			object := "object.txt"
			expiration := clock.Now(ctx).UTC().Add(7 * 24 * time.Hour)
			opts := testutil.GetSignedURLOptions(ctx)

			gsClient, err := storage.NewClient(ctx)
			So(err, ShouldBeNil)

			url, err := GenerateSignedURL(ctx, gsClient, bucket, object, expiration, opts)
			So(err, ShouldBeNil)
			So(url, ShouldStartWith, "https://storage.googleapis.com/testBucket/object.txt?X-Goog-Algorithm=GOOG4-RSA-SHA256&X-Goog-Credential=")
		})
	})
}

func TestSplit(t *testing.T) {
	t.Parallel()

	Convey(`Split`, t, func() {
		Convey(`Valid`, func() {
			bucket := "testBucket"
			object := "object.txt"
			b, o := Split(fmt.Sprintf("gs://%s/%s", bucket, object))
			So(b, ShouldEqual, bucket)
			So(o, ShouldEqual, object)
		})

		Convey(`With no object`, func() {
			bucket := "testBucket"
			b, o := Split(fmt.Sprintf("gs://%s", bucket))
			So(b, ShouldEqual, bucket)
			So(o, ShouldBeEmpty)
		})

		Convey(`With invalid prefix`, func() {
			bucket := "testBucket"
			object := "object.txt"
			path := fmt.Sprintf("xyz://%s/%s", bucket, object)
			b, o := Split(path)
			So(b, ShouldBeEmpty)
			So(o, ShouldEqual, path)
		})
	})
}
