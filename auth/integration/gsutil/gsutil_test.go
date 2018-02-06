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

package gsutil

import (
	"bufio"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
)

func TestProtocol(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)

	Convey("With server", t, func(c C) {
		stateDir, err := ioutil.TempDir("", "gsutil_auth")
		So(err, ShouldBeNil)
		defer os.RemoveAll(stateDir)

		s := Server{
			Source: oauth2.StaticTokenSource(&oauth2.Token{
				AccessToken: "tok1",
				Expiry:      clock.Now(ctx).Add(30 * time.Minute),
			}),
			StateDir: stateDir,
		}
		botoPath, err := s.Start(ctx)
		So(err, ShouldBeNil)
		defer s.Stop(ctx)

		// Parse generate .boto file.
		f, err := os.Open(botoPath)
		So(err, ShouldBeNil)
		defer f.Close()
		boto := map[string]string{}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			chunks := strings.Split(scanner.Text(), "=")
			if len(chunks) == 2 {
				boto[strings.TrimSpace(chunks[0])] = strings.TrimSpace(chunks[1])
			}
		}

		call := func(refreshTok string) (*http.Response, error) {
			form := url.Values{}
			form.Add("grant_type", "refresh_token")
			form.Add("refresh_token", refreshTok)
			form.Add("client_id", "fake_client_id")
			form.Add("client_secret", "fake_client_secret")
			return http.Post(boto["provider_token_uri"], "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
		}

		Convey("Happy path", func() {
			resp, err := call(boto["gs_oauth2_refresh_token"])
			So(err, ShouldBeNil)
			defer resp.Body.Close()
			tok := map[string]interface{}{}
			So(resp.StatusCode, ShouldEqual, 200)
			So(resp.Header.Get("Content-Type"), ShouldEqual, "application/json")
			So(json.NewDecoder(resp.Body).Decode(&tok), ShouldBeNil)
			So(tok, ShouldResemble, map[string]interface{}{
				"access_token": "tok1",
				"expires_in":   1800.0,
				"token_type":   "Bearer",
			})
		})

		Convey("Bad refresh token", func() {
			resp, err := call("bad-refresh-token")
			So(err, ShouldBeNil)
			defer resp.Body.Close()
			So(resp.StatusCode, ShouldEqual, 400)
		})
	})
}
