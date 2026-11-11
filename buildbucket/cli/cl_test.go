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

package cli

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/buildbucket/proto"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRetrieveCL(t *testing.T) {
	ftt.Run("RetrieveCL", t, func(t *ftt.Test) {
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		tc.SetTimerCallback(func(d time.Duration, _ clock.Timer) {
			tc.Add(d)
		})

		run := func(respond func(attempt int) (int, string)) (*pb.GerritChange, int, error) {
			attempts := 0
			client := &http.Client{
				Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
					attempts++
					code, body := respond(attempts)
					return &http.Response{
						StatusCode: code,
						Body:       io.NopCloser(strings.NewReader(body)),
					}, nil
				}),
			}
			cl, err := (&clsFlag{}).retrieveCL(ctx, "crrev.com/c/123/4", client, true)
			return cl, attempts, err
		}

		t.Run("retries on HTTP 429 Too Many Requests", func(t *ftt.Test) {
			actual, attempts, err := run(func(attempt int) (int, string) {
				if attempt < 3 {
					return http.StatusTooManyRequests, "rate limit exceeded"
				}
				return http.StatusOK, ")]}'\n{\"project\": \"infra/luci/luci-go\", \"_number\": 123}"
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, attempts, should.Equal(3))
			assert.Loosely(t, actual, should.Match(&pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Project:  "infra/luci/luci-go",
				Change:   123,
				Patchset: 4,
			}))
		})

		t.Run("does not retry on permanent errors (HTTP 404)", func(t *ftt.Test) {
			_, attempts, err := run(func(int) (int, string) {
				return http.StatusNotFound, "not found"
			})
			assert.Loosely(t, err, should.ErrLike("not found"))
			assert.Loosely(t, attempts, should.Equal(1))
		})
	})
}

func TestParseCL(t *testing.T) {
	ftt.Run("ParseCL", t, func(t *ftt.Test) {
		t.Run("https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1541677/7", func(t *ftt.Test) {
			actual, err := parseCL("https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1541677/7")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Match(&pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Project:  "infra/luci/luci-go",
				Change:   1541677,
				Patchset: 7,
			}))
		})

		t.Run("https://chromium-review.googlesource.com/#/c/infra/luci/luci-go/+/1541677/7", func(t *ftt.Test) {
			actual, err := parseCL("https://chromium-review.googlesource.com/#/c/infra/luci/luci-go/+/1541677/7")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Match(&pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Project:  "infra/luci/luci-go",
				Change:   1541677,
				Patchset: 7,
			}))
		})

		t.Run("https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1541677/7/buildbucket/cmd/bb/base_command.go", func(t *ftt.Test) {
			actual, err := parseCL("https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1541677/7/buildbucket/cmd/bb/base_command.go")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Match(&pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Project:  "infra/luci/luci-go",
				Change:   1541677,
				Patchset: 7,
			}))
		})

		t.Run("https://chromium-review.googlesource.com/c/1541677/7", func(t *ftt.Test) {
			actual, err := parseCL("https://chromium-review.googlesource.com/c/1541677/7")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Match(&pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Change:   1541677,
				Patchset: 7,
			}))
		})

		t.Run("https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1541677", func(t *ftt.Test) {
			actual, err := parseCL("https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1541677")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Match(&pb.GerritChange{
				Host:    "chromium-review.googlesource.com",
				Project: "infra/luci/luci-go",
				Change:  1541677,
			}))
		})

		t.Run("crrev.com/c/123", func(t *ftt.Test) {
			actual, err := parseCL("crrev.com/c/123")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Match(&pb.GerritChange{
				Host:   "chromium-review.googlesource.com",
				Change: 123,
			}))
		})

		t.Run("crrev.com/c/123/4", func(t *ftt.Test) {
			actual, err := parseCL("crrev.com/c/123/4")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Match(&pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Change:   123,
				Patchset: 4,
			}))
		})

		t.Run("crrev.com/i/123", func(t *ftt.Test) {
			actual, err := parseCL("crrev.com/i/123")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Match(&pb.GerritChange{
				Host:   "chrome-internal-review.googlesource.com",
				Change: 123,
			}))
		})

		t.Run("https://crrev.com/i/123", func(t *ftt.Test) {
			actual, err := parseCL("https://crrev.com/i/123")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Match(&pb.GerritChange{
				Host:   "chrome-internal-review.googlesource.com",
				Change: 123,
			}))
		})

		t.Run("https://chrome-internal-review.googlesource.com/c/src/+/1/2", func(t *ftt.Test) {
			actual, err := parseCL("https://chrome-internal-review.googlesource.com/c/src/+/1/2")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Match(&pb.GerritChange{
				Host:     "chrome-internal-review.googlesource.com",
				Project:  "src",
				Change:   1,
				Patchset: 2,
			}))
		})
	})
}
