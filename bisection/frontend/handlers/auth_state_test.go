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

package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.chromium.org/luci/server/router"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGetAuthState(t *testing.T) {
	t.Parallel()

	Convey("Requests which do not have the same-origin are forbidden", t, func() {
		// Set up a test router to handle auth state requests
		testRouter := router.New()
		testRouter.GET("/api/authState", nil, GetAuthState)

		// Create request
		request, err := http.NewRequest("GET", "/api/authState", nil)
		So(err, ShouldBeNil)

		// Check response status code
		response := httptest.NewRecorder()
		testRouter.ServeHTTP(response, request)
		result := response.Result()
		So(result.StatusCode, ShouldEqual, http.StatusForbidden)
	})
}
