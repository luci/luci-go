// Copyright 2023 The LUCI Authors.
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

package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/golang/mock/gomock"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	configpb "go.chromium.org/luci/milo/proto/config"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"
)

func TestRawArtifactHandler(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestRawArtifactHandler`, t, func(t *ftt.Test) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		resultdbMock := resultpb.NewMockResultDBClient(ctrl)
		srv := &HTTPService{
			GetSettings: func(c context.Context) (*configpb.Settings, error) {
				return &configpb.Settings{
					Resultdb: &configpb.Settings_ResultDB{
						Host: "test.result.api.cr.dev",
					},
				}, nil
			},
			GetResultDBClient: func(c context.Context, host string, as auth.RPCAuthorityKind) (resultpb.ResultDBClient, error) {
				assert.Loosely(t, host, should.Equal("test.result.api.cr.dev"))
				assert.Loosely(t, as, should.Equal(auth.AsSessionUser))
				return resultdbMock, nil
			},
		}

		w := httptest.NewRecorder()
		c := context.Background()
		c = auth.WithState(c, &authtest.FakeState{Identity: identity.AnonymousIdentity})

		handler := srv.buildRawArtifactHandler("/prefix/")

		t.Run(`without encoded character`, func(t *ftt.Test) {
			resultdbMock.
				EXPECT().
				GetArtifact(
					gomock.Any(),
					&resultpb.GetArtifactRequest{
						Name: "test/artifact/name",
					},
				).
				Return(&resultpb.Artifact{
					FetchUrl: "https://test.com/artifact/fetch/url",
				}, nil)

			url, err := url.Parse("/prefix/test/artifact/name?n=50000")
			assert.Loosely(t, err, should.BeNil)

			req := &http.Request{URL: url}
			w := httptest.NewRecorder()
			c := context.Background()
			c = auth.WithState(c, &authtest.FakeState{Identity: identity.AnonymousIdentity})
			ctx := &router.Context{
				Request: req.WithContext(c),
				Writer:  w,
			}

			err = handler(ctx)
			assert.Loosely(t, err, should.BeNil)

			res := w.Result()
			assert.Loosely(t, res.StatusCode, should.Equal(http.StatusFound))
			assert.Loosely(t, res.Header.Get("location"), should.Equal("https://test.com/artifact/fetch/url?n=50000"))
		})

		t.Run(`with encoded character`, func(t *ftt.Test) {
			resultdbMock.
				EXPECT().
				GetArtifact(
					gomock.Any(),
					&resultpb.GetArtifactRequest{
						Name: "test/artifact%2fname",
					},
				).
				Return(&resultpb.Artifact{
					FetchUrl: "https://test.com/artifact/fetch/url",
				}, nil)

			url, err := url.Parse("/prefix/test/artifact%2fname?n=50000")
			assert.Loosely(t, url.RawPath, should.NotBeEmpty)
			assert.Loosely(t, err, should.BeNil)

			req := &http.Request{URL: url}
			ctx := &router.Context{
				Request: req.WithContext(c),
				Writer:  w,
			}

			err = handler(ctx)
			assert.Loosely(t, err, should.BeNil)

			res := w.Result()
			assert.Loosely(t, res.StatusCode, should.Equal(http.StatusFound))
			assert.Loosely(t, res.Header.Get("location"), should.Equal("https://test.com/artifact/fetch/url?n=50000"))
		})

		t.Run(`with encoded character but the URL.RawPath is not set`, func(t *ftt.Test) {
			resultdbMock.
				EXPECT().
				GetArtifact(
					gomock.Any(),
					&resultpb.GetArtifactRequest{
						Name: "test/artifact%20name",
					},
				).
				Return(&resultpb.Artifact{
					FetchUrl: "https://test.com/artifact/fetch/url",
				}, nil)

			url, err := url.Parse("/prefix/test/artifact%20name?n=50000")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, url.RawPath, should.BeEmpty)

			req := &http.Request{URL: url}
			ctx := &router.Context{
				Request: req.WithContext(c),
				Writer:  w,
			}

			err = handler(ctx)
			assert.Loosely(t, err, should.BeNil)

			res := w.Result()
			assert.Loosely(t, res.StatusCode, should.Equal(http.StatusFound))
			assert.Loosely(t, res.Header.Get("location"), should.Equal("https://test.com/artifact/fetch/url?n=50000"))
		})

		t.Run(`with existing query search params`, func(t *ftt.Test) {
			resultdbMock.
				EXPECT().
				GetArtifact(
					gomock.Any(),
					&resultpb.GetArtifactRequest{
						Name: "test/artifact/name",
					},
				).
				Return(&resultpb.Artifact{
					FetchUrl: "https://test.com/artifact/fetch/url?token=_12345",
				}, nil)

			url, err := url.Parse("/prefix/test/artifact/name?n=50000")
			assert.Loosely(t, err, should.BeNil)

			req := &http.Request{URL: url}
			w := httptest.NewRecorder()
			c := context.Background()
			c = auth.WithState(c, &authtest.FakeState{Identity: identity.AnonymousIdentity})
			ctx := &router.Context{
				Request: req.WithContext(c),
				Writer:  w,
			}

			err = handler(ctx)
			assert.Loosely(t, err, should.BeNil)

			res := w.Result()
			assert.Loosely(t, res.StatusCode, should.Equal(http.StatusFound))
			assert.Loosely(t, res.Header.Get("location"), should.Equal("https://test.com/artifact/fetch/url?n=50000&token=_12345"))
		})
	})
}
