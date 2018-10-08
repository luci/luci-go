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

package googleoauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
)

type token struct {
	AccessToken string `json:"accessToken"`
	ExpireTime  string `json:"expireTime"`
	TokenType   string
}

func TestGetAccessToken(t *testing.T) {
	ctx, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeLocal)

	Convey("Happy path", t, func() {
		expireTime, err := time.Parse(time.RFC3339, clock.Now(ctx).Add(time.Hour).UTC().Format(time.RFC3339))
		So(err, ShouldBeNil)
		_, tok, _, err := call(ctx, GenTokenFlowParams{
			ServiceAccount: "account@example.com",
			Scopes:         []string{"a", "b"},
		}, 200, token{"abc", expireTime.Format(time.RFC3339), "Bearer"})
		So(err, ShouldBeNil)

		// Response is understood.
		So(tok, ShouldResemble, &oauth2.Token{
			AccessToken: "abc",
			TokenType:   "Bearer",
			Expiry:      expireTime,
		})
	})

	Convey("Uses Bearer as default", t, func() {
		expireTime, err := time.Parse(time.RFC3339, clock.Now(ctx).Add(time.Hour).UTC().Format(time.RFC3339))
		So(err, ShouldBeNil)
		_, tok, _, err := call(ctx, GenTokenFlowParams{
			ServiceAccount: "account@example.com",
			Scopes:         []string{"a", "b"},
		}, 200, token{"def", expireTime.Format(time.RFC3339), ""})

		So(err, ShouldBeNil)
		So(tok, ShouldResemble, &oauth2.Token{
			AccessToken: "def",
			TokenType:   "Bearer",
			Expiry:      expireTime,
		})
	})

	Convey("Bad HTTP code", t, func() {
		_, _, _, err := call(ctx, GenTokenFlowParams{
			ServiceAccount: "account@example.com",
			Scopes:         []string{"a", "b"},
		}, 403, nil)
		So(err, ShouldHaveSameTypeAs, &googleapi.Error{})
		So(err.(*googleapi.Error).Code, ShouldEqual, 403)
	})

	Convey("Not valid JSON", t, func() {
		_, _, _, err := call(ctx, GenTokenFlowParams{
			ServiceAccount: "account@example.com",
			Scopes:         []string{"a", "b"},
		}, 200, "zzzzzz")
		So(err, ShouldNotBeNil)
	})
}

func call(ctx context.Context, params GenTokenFlowParams, status int, resp interface{}) (url.Values, *oauth2.Token, string, error) {
	values := make(chan url.Values, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			panic("not a POST")
		}
		err := r.ParseForm()
		if err != nil {
			panic(err)
		}
		values <- r.Form
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	params.tokenEndpoint = ts.URL

	tok, err := GetAccessToken(ctx, params)
	req := <-values
	return req, tok, ts.URL, err
}
