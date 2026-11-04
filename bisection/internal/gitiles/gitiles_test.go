// Copyright 2026 The LUCI Authors.
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

package gitiles

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/bisection/util"
)

type flakyGitilesClient struct {
	calls     int
	responses []struct {
		data string
		err  error
	}
}

func (f *flakyGitilesClient) sendRequest(c context.Context, url string, params map[string]string) (string, error) {
	idx := f.calls
	f.calls++
	if idx >= len(f.responses) {
		idx = len(f.responses) - 1
	}
	return f.responses[idx].data, f.responses[idx].err
}

func TestGetChangeLogs(t *testing.T) {
	t.Parallel()

	ftt.Run("GetChangeLogs", t, func(t *ftt.Test) {
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		tc.SetTimerCallback(func(d time.Duration, tmr clock.Timer) {
			tc.Add(d)
		})

		validResp := ")]}'\n" + `{"log":[{"commit":"abc123"}]}`

		t.Run("retries transient 504 and succeeds", func(t *ftt.Test) {
			client := &flakyGitilesClient{
				responses: []struct {
					data string
					err  error
				}{
					{err: transient.Tag.Apply(fmt.Errorf("Bad response code: 504"))},
					{err: transient.Tag.Apply(fmt.Errorf("Bad response code: 504"))},
					{data: validResp},
				},
			}
			c := context.WithValue(ctx, MockedGitilesClientKey, client)
			logs, err := GetChangeLogs(c, "https://chromium.googlesource.com/chromium/src", "rev1", "rev2")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(logs), should.Equal(1))
			assert.Loosely(t, logs[0].Commit, should.Equal("abc123"))
			assert.Loosely(t, client.calls, should.Equal(3))
		})

		t.Run("retries 404 replication lag and succeeds", func(t *ftt.Test) {
			client := &flakyGitilesClient{
				responses: []struct {
					data string
					err  error
				}{
					{err: util.HTTPStatusTag.ApplyValue(fmt.Errorf("Bad response code: 404"), http.StatusNotFound)},
					{data: validResp},
				},
			}
			c := context.WithValue(ctx, MockedGitilesClientKey, client)
			logs, err := GetChangeLogs(c, "https://chromium.googlesource.com/chromium/src", "rev1", "rev2")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(logs), should.Equal(1))
			assert.Loosely(t, client.calls, should.Equal(2))
		})

		t.Run("does not retry non-transient 403", func(t *ftt.Test) {
			client := &flakyGitilesClient{
				responses: []struct {
					data string
					err  error
				}{
					{err: util.HTTPStatusTag.ApplyValue(fmt.Errorf("Bad response code: 403"), http.StatusForbidden)},
					{data: validResp},
				},
			}
			c := context.WithValue(ctx, MockedGitilesClientKey, client)
			_, err := GetChangeLogs(c, "https://chromium.googlesource.com/chromium/src", "rev1", "rev2")
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, client.calls, should.Equal(1))
		})

		t.Run("returns error after exhausting retries", func(t *ftt.Test) {
			client := &flakyGitilesClient{
				responses: []struct {
					data string
					err  error
				}{
					{err: util.HTTPStatusTag.ApplyValue(fmt.Errorf("Bad response code: 404"), http.StatusNotFound)},
				},
			}
			c := context.WithValue(ctx, MockedGitilesClientKey, client)
			_, err := GetChangeLogs(c, "https://chromium.googlesource.com/chromium/src", "rev1", "rev2")
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, client.calls, should.Equal(11))
		})
	})
}
