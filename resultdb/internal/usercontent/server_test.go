// Copyright 2020 The LUCI Authors.
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

package usercontent

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets/testsecrets"

	"go.chromium.org/luci/resultdb/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestIsolate(t *testing.T) {
	t.Parallel()
	Convey(`TestIsolate`, t, func(c C) {
		ctx := testutil.TestingContext()

		ctx = authtest.MockAuthConfig(ctx)
		authState := &authtest.FakeState{}
		ctx = auth.WithState(ctx, authState)
		ctx = testsecrets.Use(ctx)

		s, err := NewServer(ctx, false, "resultdb-usercontent.example.com")
		s.testFetchIsolate = func(ctx context.Context, isolateHost, ns, digest string, w io.Writer) error {
			return fmt.Errorf("unexpected")
		}
		So(err, ShouldBeNil)

		r := router.NewWithRootContext(ctx)
		s.InstallHandlers(r)

		fetch := func(url string) (status int, contents string) {
			// Then request the content.
			contentReq, err := http.NewRequest("GET", url, nil)
			So(err, ShouldBeNil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, contentReq)
			res := rec.Result()
			rawContents, err := ioutil.ReadAll(res.Body)
			So(err, ShouldBeNil)
			return res.StatusCode, string(rawContents)
		}

		Convey(`URL sanity check`, func() {
			// Validate generated signed URL.
			u, _, err := s.GenerateSignedIsolateURL(ctx, "isolate.example.com", "default-gzip", "deadbeef")
			So(err, ShouldBeNil)

			So(u.Query().Get("token"), ShouldNotEqual, "")
			u.RawQuery = ""
			So(u.String(), ShouldEqual, "https://resultdb-usercontent.example.com/isolate/isolate.example.com/default-gzip/deadbeef")
		})

		Convey(`No token`, func() {
			status, _ := fetch("https://resultdb-usercontent.example.com/isolate/isolate.example.com/default-gzip/deadbeef")
			So(status, ShouldEqual, http.StatusUnauthorized)
		})

		Convey(`Invalid token`, func() {
			status, _ := fetch("https://resultdb-usercontent.example.com/isolate/isolate.example.com/default-gzip/deadbeef?token=bad")
			So(status, ShouldEqual, http.StatusForbidden)
		})

		Convey(`E2E`, func() {
			s.testFetchIsolate = func(ctx context.Context, isolateHost, ns, digest string, w io.Writer) error {
				_, err := w.Write([]byte("contents"))
				return err
			}
			u, exp, err := s.GenerateSignedIsolateURL(ctx, "isolate.example.com", "default-gzip", "deadbeef")
			So(err, ShouldBeNil)
			So(exp, ShouldResemble, clock.Now(ctx).UTC().Add(time.Hour))

			status, actualContents := fetch(u.String())
			So(status, ShouldEqual, http.StatusOK)
			So(actualContents, ShouldEqual, "contents")
		})
	})
}
