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

package server

import (
	"context"
	"net/http"
	"testing"

	"golang.org/x/oauth2"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGetUserCredentials(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	m := OAuth2Method{}

	call := func(hdr string) (*oauth2.Token, error) {
		return m.GetUserCredentials(ctx, &http.Request{Header: http.Header{
			"Authorization": {hdr},
		}})
	}

	Convey("Works", t, func() {
		tok, err := call("Bearer abc.def")
		So(err, ShouldBeNil)
		So(tok, ShouldResemble, &oauth2.Token{
			AccessToken: "abc.def",
			TokenType:   "Bearer",
		})
	})

	Convey("Bad headers", t, func() {
		_, err := call("")
		So(err, ShouldEqual, errBadAuthHeader)
		_, err = call("abc.def")
		So(err, ShouldEqual, errBadAuthHeader)
		_, err = call("Basic abc.def")
		So(err, ShouldEqual, errBadAuthHeader)
	})
}
