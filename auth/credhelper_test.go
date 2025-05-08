// Copyright 2025 The LUCI Authors.
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

package auth

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/auth/credhelperpb"
	"go.chromium.org/luci/auth/internal"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/exec/execmock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestCredentialHelper(t *testing.T) {
	t.Parallel()

	testTime := time.Date(2044, time.February, 3, 4, 5, 0, 0, time.UTC)
	testExpiry := testTime.Add(30 * time.Minute)

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx, _ := testclock.UseTime(context.Background(), testTime)
		ctx = execmock.Init(ctx)

		p := &credHelperTokenProvider{
			cfg: &credhelperpb.Config{
				Protocol: credhelperpb.Protocol_RECLIENT,
				Exec:     "some-exe",
				Args:     []string{"--some-arg=1"},
			},
			handler: reclientHandler,
		}

		t.Run("CacheKey", func(t *ftt.Test) {
			k, err := p.CacheKey(ctx)
			assert.NoErr(t, err)
			assert.That(t, k, should.Match(&internal.CacheKey{
				Key: "credhelper/reclient/8bd2e226ad4305619e083e5ebd43017d635ec2ca3e9d006dbbd22849f91f7ea5",
			}))
		})

		t.Run("OK", func(t *ftt.Test) {
			uses := execmock.Simple.Mock(ctx, execmock.SimpleInput{
				Stdout: fmt.Sprintf(`{
					"headers": {
						"Authorization": "Bearer auth-token"
					},
					"token":"auth-token",
					"expiry": %q
				}`, testExpiry.Format(time.UnixDate)),
			})

			tok, err := p.MintToken(ctx, nil)
			assert.NoErr(t, err)
			assert.That(t, tok.AccessToken, should.Equal("auth-token"))
			assert.That(t, tok.Expiry, should.Match(testExpiry))

			calls := uses.Snapshot()
			assert.That(t, calls[0].Args[1:], should.Match([]string{"--some-arg=1"}))
		})

		t.Run("Exec fail", func(t *ftt.Test) {
			execmock.Simple.Mock(ctx, execmock.SimpleInput{
				ExitCode: 2,
			})
			_, err := p.MintToken(ctx, nil)
			assert.That(t, err, should.ErrLike("running the credential helper: exit status 2"))
		})

		t.Run("Bad stdout", func(t *ftt.Test) {
			execmock.Simple.Mock(ctx, execmock.SimpleInput{
				Stdout: "not JSON",
			})
			_, err := p.MintToken(ctx, nil)
			assert.That(t, err, should.ErrLike("parsing the credential helper output: not a valid JSON output format"))
		})

		t.Run("Expired token", func(t *ftt.Test) {
			execmock.Simple.Mock(ctx, execmock.SimpleInput{
				Stdout: fmt.Sprintf(`{
					"headers": {
						"Authorization": "Bearer auth-token"
					},
					"token":"auth-token",
					"expiry": %q
				}`, testTime.Add(-time.Second).Format(time.UnixDate)),
			})
			_, err := p.MintToken(ctx, nil)
			assert.That(t, err, should.ErrLike("the credential helper returned expired token (it expired 1s ago)"))
		})
	})
}

func TestReclientHandler(t *testing.T) {
	t.Parallel()

	testExpiry := time.Date(2025, time.May, 1, 15, 8, 49, 0, time.UTC)

	cases := []struct {
		stdout string
		err    any
		token  string
		expiry time.Time
	}{
		// OK.
		{
			stdout: `{
				"headers": {
					"Authorization": "Bearer auth-token"
				},
				"token":"auth-token",
				"expiry": "Thu May  1 15:08:49 UTC 2025"
			}`,
			token:  "auth-token",
			expiry: testExpiry,
		},
		{
			stdout: `{
				"headers": {},
				"token":"auth-token",
				"expiry": "Thu May  1 15:08:49 UTC 2025"
			}`,
			token:  "auth-token",
			expiry: testExpiry,
		},
		{
			stdout: `{
				"token":"auth-token",
				"expiry": "Thu May  1 15:08:49 UTC 2025"
			}`,
			token:  "auth-token",
			expiry: testExpiry,
		},

		// Fails.
		{
			stdout: "not JSON",
			err:    "not a valid JSON output format",
		},
		{
			stdout: `{
				"headers": {
					"Authorization": "Bearer auth-token"
				},
				"expiry": "Thu May  1 15:08:49 UTC 2025"
			}`,
			err: "no \"token\" field in the output",
		},
		{
			stdout: `{
				"headers": {
					"Authorization": "Bearer auth-token"
				},
				"token":"auth-token"
			}`,
			err: "no \"expiry\" field in the output",
		},
		{
			stdout: `{
				"headers": {
					"Blah": "value"
				},
				"token":"auth-token",
				"expiry": "Thu May  1 15:08:49 UTC 2025"
			}`,
			err: "no \"Authorization\" header in the output",
		},
		{
			stdout: `{
				"headers": {
					"Authorization": "Bearer different-token"
				},
				"token":"auth-token",
				"expiry": "Thu May  1 15:08:49 UTC 2025"
			}`,
			err: "\"Authorization\" header value doesn't match the token",
		},
		{
			stdout: `{
				"headers": {
					"Authorization": "Bearer auth-token",
					"Extra": "value"
				},
				"token":"auth-token",
				"expiry": "Thu May  1 15:08:49 UTC 2025"
			}`,
			err: "the output has headers other than \"Authorization\", this is not supported",
		},
		{
			stdout: `{
				"headers": {
					"Authorization": "Bearer auth-token"
				},
				"token":"auth-token",
				"expiry": "123456"
			}`,
			err: "bad expiry time format \"123456\", must match \"Mon Jan _2 15:04:05 MST 2006\"",
		},
	}

	for _, c := range cases {
		tok, err := reclientHandler([]byte(c.stdout))
		if c.err != nil {
			assert.That(t, err, should.ErrLike(c.err))
		} else {
			assert.NoErr(t, err)
			assert.That(t, tok.AccessToken, should.Equal(c.token))
			assert.That(t, tok.Expiry, should.Match(c.expiry))
		}
	}
}
