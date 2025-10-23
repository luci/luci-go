// Copyright 2017 The LUCI Authors.
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

package cas

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/cipd/appengine/impl/testutil"
)

func TestGetSignedURL(t *testing.T) {
	t.Parallel()

	ftt.Run("with context", t, func(t *ftt.Test) {
		ctx := caching.WithEmptyProcessCache(context.Background())
		ctx, cl := testclock.UseTime(ctx, testclock.TestRecentTimeUTC)

		var signed []byte
		var signature string
		var signErr error
		signer := func(context.Context) (*signer, error) {
			return &signer{
				Email: "test@example.com",
				SignBytes: func(_ context.Context, data []byte) (key string, sig []byte, err error) {
					signed = data
					return "", []byte(signature), signErr
				},
			}, nil
		}

		t.Run("Works", func(t *ftt.Test) {
			signature = "sig1"
			url1, size, err := getSignedURL(ctx, signer, &mockedSignerGS{exists: true}, &signedURLParams{
				GsPath: "/bucket/path",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, url1, should.HavePrefix("https://storage.googleapis.com/bucket/path?"))
			assert.That(t, size, should.Equal[uint64](123))
			assert.Loosely(t, string(signed), should.HavePrefix("GOOG4-RSA-SHA256"))

			// 1h later returns the same cached URL.
			cl.Add(time.Hour)

			signature = "sig2" // must not be picked up
			url2, _, err := getSignedURL(ctx, signer, &mockedSignerGS{exists: true}, &signedURLParams{
				GsPath: "/bucket/path",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, url2, should.Equal(url1))

			// 31min later the cache expires and new link is generated.
			cl.Add(31 * time.Minute)

			signature = "sig3"
			url3, _, err := getSignedURL(ctx, signer, &mockedSignerGS{exists: true}, &signedURLParams{
				GsPath: "/bucket/path",
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, url3, should.NotEqual(url1))
		})

		t.Run("Absence is cached", func(t *ftt.Test) {
			gs := &mockedSignerGS{exists: false}
			_, _, err := getSignedURL(ctx, signer, gs, &signedURLParams{
				GsPath: "/bucket/path",
			})
			assert.Loosely(t, err, should.ErrLike("doesn't exist"))
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
			assert.Loosely(t, gs.calls, should.Equal(1))

			// 30 sec later same check is reused.
			cl.Add(30 * time.Second)
			_, _, err = getSignedURL(ctx, signer, gs, &signedURLParams{
				GsPath: "/bucket/path",
			})
			assert.Loosely(t, err, should.ErrLike("doesn't exist"))
			assert.Loosely(t, gs.calls, should.Equal(1))

			// 31 sec later the cache expires and new check is made.
			cl.Add(31 * time.Second)
			_, _, err = getSignedURL(ctx, signer, gs, &signedURLParams{
				GsPath: "/bucket/path",
			})
			assert.Loosely(t, err, should.ErrLike("doesn't exist"))
			assert.Loosely(t, gs.calls, should.Equal(2))
		})

		t.Run("Signing error", func(t *ftt.Test) {
			signErr = fmt.Errorf("boo, error")
			_, _, err := getSignedURL(ctx, signer, &mockedSignerGS{exists: true}, &signedURLParams{
				GsPath: "/bucket/path",
			})
			assert.Loosely(t, err, should.ErrLike("boo, error"))
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.Internal))
		})

		t.Run("Content-Disposition", func(t *ftt.Test) {
			signature = "sig1"
			url, _, _ := getSignedURL(ctx, signer, &mockedSignerGS{exists: true}, &signedURLParams{
				GsPath:   "/bucket/path",
				Filename: "name.txt",
			})
			assert.Loosely(t, url, should.ContainSubstring("response-content-disposition=attachment%3B+filename%3D%22name.txt%22"))
		})
	})
}

type mockedSignerGS struct {
	testutil.NoopGoogleStorage

	exists bool
	calls  int
}

func (m *mockedSignerGS) Size(ctx context.Context, path string) (size uint64, exists bool, err error) {
	m.calls++
	return 123, m.exists, nil
}
