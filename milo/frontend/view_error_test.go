// Copyright 2018 The LUCI Authors.
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
	"bytes"
	"net/http/httptest"
	"testing"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/server/router"

	. "github.com/smartystreets/goconvey/convey"
)

func TestViewError(t *testing.T) {
	t.Parallel()

	Convey(`TestErrorHandler`, t, func() {
		r := httptest.NewRecorder()
		ctx := &router.Context{
			Context: gaetesting.TestingContextWithAppID("luci-milo-dev"),
			Writer:  r,
			Request: httptest.NewRequest("GET", "/url", bytes.NewReader(nil)),
		}

		Convey("redirects to login page on authorized access", func() {
			err := errors.Reason("").Tag(common.CodeUnauthorized).Err()
			ErrorHandler(ctx, err)
			So(r.Code, ShouldEqual, 302)
		})
	})
}
