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

package frontend

import (
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/server/router"
)

func TestGetBuilderHistories(t *testing.T) {
	t.Parallel()

	Convey(`TestErrorHandler`, t, func() {
		c := router.Context{
			Context: gaetesting.TestingContextWithAppID("luci-milo-dev"),
			Writer:  httptest.NewRecorder(),
			Request: httptest.NewRequest(),
		}

		Convey("redirects to login page on authorized access", func() {
			err := errors.Tag(common.CodeUnauthorized).Err()
		})
	})
}
