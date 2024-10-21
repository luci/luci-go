// Copyright 2019 The LUCI Authors.
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

package host

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/compute/metadata"
	"golang.org/x/oauth2"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/integration/authtest"
	"go.chromium.org/luci/auth/integration/localauth"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/lucictx"
)

// Use this instead of ShouldStartWith to avoid logging a real token if the
// assertion fails.
//
// If the fake auth and/or this package don't work as intended, it's possible
// that the real ambient Swarming auth could leak through to the test. Using
// ShouldStartWith would, on an assertion failure, print the 'actual' value
// which would be a real authentication token, should the context leak.
func tokenShouldStartWith(actual any, expected ...any) string {
	if len(expected) != 1 {
		panic(errors.Reason("tokenShouldStartWith takes one expected value, got %d", len(expected)).Err())
	}

	if expectedStr := expected[0].(string); !strings.HasPrefix(actual.(string), expectedStr) {
		return fmt.Sprintf("Expected <token> to start with %q", expectedStr)
	}
	return ""
}

func testCtx(t testing.TB) (context.Context, func()) {
	t.Helper()
	ctx := context.Background()
	if testing.Verbose() {
		ctx = logging.SetLevel(gologger.StdConfig.Use(ctx), logging.Debug)
	}

	// Setup a fake local auth. It is a real localhost server, it needs real
	// clock, so set it up before mocking time.
	fakeAuth := localauth.Server{
		TokenGenerators: map[string]localauth.TokenGenerator{
			"task": &authtest.FakeTokenGenerator{
				Email:  "task@example.com",
				Prefix: "task_token_",
			},
		},
		DefaultAccountID: "task",
	}
	la, err := fakeAuth.Start(ctx)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	ctx = lucictx.SetLocalAuth(ctx, la)

	return ctx, func() { fakeAuth.Stop(ctx) }
}

func TestAuth(t *testing.T) {
	ftt.Run(`test auth environment`, t, func(t *ftt.Test) {
		ctx, closer := testCtx(t)
		defer closer()

		t.Run(`default`, func(c *ftt.Test) {
			ch, err := Run(ctx, nil, func(ctx context.Context, _ Options, _ <-chan lucictx.DeadlineEvent, _ func()) {
				c.Run("Task account is available", func(c *ftt.Test) {
					a := auth.NewAuthenticator(ctx, auth.SilentLogin, auth.Options{
						Method: auth.LUCIContextMethod,
					})
					email, err := a.GetEmail()
					assert.Loosely(c, err, should.BeNil)
					assert.Loosely(c, email, should.Equal("task@example.com"))
					tok, err := a.GetAccessToken(time.Minute)
					assert.Loosely(c, err, should.BeNil)
					assert.Loosely(c, tok.AccessToken, convey.Adapt(tokenShouldStartWith)("task_token_"))
				})

				c.Run("Git config is set", func(c *ftt.Test) {
					gitHome := os.Getenv("INFRA_GIT_WRAPPER_HOME")
					assert.Loosely(c, gitHome, should.NotEqual(""))

					cfg, err := os.ReadFile(filepath.Join(gitHome, ".gitconfig"))
					assert.Loosely(c, err, should.BeNil)

					assert.Loosely(c, string(cfg), should.ContainSubstring("email = task@example.com"))
					assert.Loosely(c, string(cfg), should.ContainSubstring("helper = luci"))
				})

				c.Run("GCE metadata server is faked", func(c *ftt.Test) {
					// Note: metadata.OnGCE() and other top-level functions in `metadata`
					// package are unreliable in this test since their values may be
					// cached in the process global memory the first time they are called
					// and luciexe call them prior to the test.
					//
					// metadata.NewClient doesn't use any process-global caches and picks
					// up the GCE_METADATA_HOST env var change done by the test.
					md := metadata.NewClient(nil)

					email, err := md.Email("default")
					assert.Loosely(c, err, should.BeNil)
					assert.Loosely(c, email, should.Equal("task@example.com"))

					tokBody, err := md.Get("instance/service-accounts/default/token")
					assert.Loosely(c, err, should.BeNil)

					tok := &oauth2.Token{}
					assert.Loosely(c, json.Unmarshal([]byte(tokBody), &tok), should.BeNil)
					assert.Loosely(c, tok.AccessToken, convey.Adapt(tokenShouldStartWith)("task_token_"))
				})
			})
			assert.Loosely(c, err, should.BeNil)
			for range ch {
				// TODO(iannucci): check for Build object contents
			}
		})
	})
}
