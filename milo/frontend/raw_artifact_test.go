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

package frontend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/golang/mock/gomock"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/auth/identity"
	configpb "go.chromium.org/luci/milo/proto/config"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"
)

func TestRawArtifactHandler(t *testing.T) {
	t.Parallel()
	Convey(`TestRawArtifactHandler`, t, func() {
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
				So(host, ShouldEqual, "test.result.api.cr.dev")
				So(as, ShouldEqual, auth.AsSessionUser)
				return resultdbMock, nil
			},
		}

		w := httptest.NewRecorder()
		c := context.Background()
		c = auth.WithState(c, &authtest.FakeState{Identity: identity.AnonymousIdentity})

		handler := srv.buildRawArtifactHandler("/prefix/")

		Convey(`without encoded character`, func() {
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
			So(err, ShouldBeNil)

			req := &http.Request{URL: url}
			w := httptest.NewRecorder()
			c := context.Background()
			c = auth.WithState(c, &authtest.FakeState{Identity: identity.AnonymousIdentity})
			ctx := &router.Context{
				Request: req.WithContext(c),
				Writer:  w,
			}

			err = handler(ctx)
			So(err, ShouldBeNil)

			res := w.Result()
			So(res.StatusCode, ShouldEqual, http.StatusFound)
			So(res.Header.Get("location"), ShouldEqual, "https://test.com/artifact/fetch/url?n=50000")
		})

		Convey(`with encoded character`, func() {
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
			So(url.RawPath, ShouldNotBeEmpty)
			So(err, ShouldBeNil)

			req := &http.Request{URL: url}
			ctx := &router.Context{
				Request: req.WithContext(c),
				Writer:  w,
			}

			err = handler(ctx)
			So(err, ShouldBeNil)

			res := w.Result()
			So(res.StatusCode, ShouldEqual, http.StatusFound)
			So(res.Header.Get("location"), ShouldEqual, "https://test.com/artifact/fetch/url?n=50000")
		})

		Convey(`with encoded character but the URL.RawPath is not set`, func() {
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
			So(err, ShouldBeNil)
			So(url.RawPath, ShouldBeEmpty)

			req := &http.Request{URL: url}
			ctx := &router.Context{
				Request: req.WithContext(c),
				Writer:  w,
			}

			err = handler(ctx)
			So(err, ShouldBeNil)

			res := w.Result()
			So(res.StatusCode, ShouldEqual, http.StatusFound)
			So(res.Header.Get("location"), ShouldEqual, "https://test.com/artifact/fetch/url?n=50000")
		})

		Convey(`with existing query search params`, func() {
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
			So(err, ShouldBeNil)

			req := &http.Request{URL: url}
			w := httptest.NewRecorder()
			c := context.Background()
			c = auth.WithState(c, &authtest.FakeState{Identity: identity.AnonymousIdentity})
			ctx := &router.Context{
				Request: req.WithContext(c),
				Writer:  w,
			}

			err = handler(ctx)
			So(err, ShouldBeNil)

			res := w.Result()
			So(res.StatusCode, ShouldEqual, http.StatusFound)
			So(res.Header.Get("location"), ShouldEqual, "https://test.com/artifact/fetch/url?n=50000&token=_12345")
		})
	})
}
