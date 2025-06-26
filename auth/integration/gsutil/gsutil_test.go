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
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestProtocol(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)

	ftt.Run("With server", t, func(c *ftt.Test) {
		stateDir, err := os.MkdirTemp("", "gsutil_auth")
		assert.Loosely(c, err, should.BeNil)
		defer os.RemoveAll(stateDir)

		s := Server{
			Source: oauth2.StaticTokenSource(&oauth2.Token{
				AccessToken: "tok1",
				Expiry:      clock.Now(ctx).Add(30 * time.Minute),
			}),
			StateDir: stateDir,
		}
		botoPath, err := s.Start(ctx)
		assert.Loosely(c, err, should.BeNil)
		defer s.Stop(ctx)

		// Parse generate .boto file.
		f, err := os.Open(botoPath)
		assert.Loosely(c, err, should.BeNil)
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

		c.Run("Happy path", func(c *ftt.Test) {
			resp, err := call(boto["gs_oauth2_refresh_token"])
			assert.Loosely(c, err, should.BeNil)
			defer resp.Body.Close()
			tok := map[string]any{}
			assert.Loosely(c, resp.StatusCode, should.Equal(200))
			assert.Loosely(c, resp.Header.Get("Content-Type"), should.Equal("application/json"))
			assert.Loosely(c, json.NewDecoder(resp.Body).Decode(&tok), should.BeNil)
			assert.Loosely(c, tok, should.Match(map[string]any{
				"access_token": "tok1",
				"expires_in":   1800.0,
				"token_type":   "Bearer",
			}))
		})

		c.Run("Bad refresh token", func(c *ftt.Test) {
			resp, err := call("bad-refresh-token")
			assert.Loosely(c, err, should.BeNil)
			defer resp.Body.Close()
			assert.Loosely(c, resp.StatusCode, should.Equal(400))
		})
	})
}
