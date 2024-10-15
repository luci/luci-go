// Copyright 2015 The LUCI Authors.
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

package httpmitm

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type record struct {
	o Origin
	d string
	e error
}

// checkRecorded tests whether a captured record (actual) matches the expected result:
// 0) Origin
// 1) bool (are we expecting an error?)
// 2) string (optional). If present, the regexp pattern that must match the data.
func checkRecorded(t testing.TB, actual *record, expected Origin, expectErr bool, expectRe ...string) {
	t.Helper()

	assert.That(t, actual.o, should.Match(expected), truth.LineContext())
	if expectErr {
		assert.Loosely(t, actual.e, should.NotBeNil, truth.LineContext())
	} else {
		assert.That(t, actual.e, should.ErrLike(nil), truth.LineContext())
	}
	for _, re := range expectRe {
		assert.That(t, actual.d, should.MatchRegexp(re), truth.LineContext())
	}
}

func TestTransport(t *testing.T) {
	t.Parallel()

	ftt.Run(`A debug HTTP client`, t, func(t *ftt.Test) {
		// Generic callback-based server. Each test will set its callback.
		var callback func() (string, error)
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			err := errors.New("No Callback.")
			if callback != nil {
				var content string
				content, err = callback()
				if err == nil {
					_, err = w.Write([]byte(content))
				}
			}
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		}))
		defer ts.Close()

		var records []*record
		client := http.Client{
			Transport: &Transport{
				Callback: func(o Origin, data []byte, err error) {
					records = append(records, &record{o, string(data), err})
				},
			},
		}

		t.Run(`Successfully fetches content.`, func(t *ftt.Test) {
			callback = func() (string, error) {
				return "Hello, client!", nil
			}
			resp, err := client.Post(ts.URL, "test", bytes.NewBufferString("DATA"))
			assert.Loosely(t, err, should.BeNil)
			defer resp.Body.Close()

			assert.Loosely(t, len(records), should.Equal(2))
			checkRecorded(t, records[0], Request, false, "(?s:POST / HTTP/1.1\r\n(.+:.+\r\n)*\r\nDATA)")
			checkRecorded(t, records[1], Response, false, "(?s:HTTP/1.1 200 OK\r\n(.+:.+\r\n)*\r\nHello, client!)")

			body, err := io.ReadAll(resp.Body)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(body), should.Equal("Hello, client!"))
			assert.Loosely(t, resp.StatusCode, should.Equal(http.StatusOK))
		})

		t.Run(`Handles HTTP error.`, func(t *ftt.Test) {
			callback = func() (string, error) {
				return "", errors.New("Failure!")
			}
			resp, err := client.Post(ts.URL, "test", bytes.NewBufferString("DATA"))
			assert.Loosely(t, err, should.BeNil)
			defer resp.Body.Close()

			assert.Loosely(t, len(records), should.Equal(2))
			checkRecorded(t, records[0], Request, false, "(?s:POST / HTTP/1.1\r\n(.+:.+\r\n)*\r\nDATA)")
			checkRecorded(t, records[1], Response, false, "(?s:HTTP/1.1 500 Internal Server Error\r\n(.+:.+\r\n)*\r\nFailure!)")

			body, err := io.ReadAll(resp.Body)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(body), should.Equal("Failure!\n"))
			assert.Loosely(t, resp.StatusCode, should.Equal(http.StatusInternalServerError))
		})

		t.Run(`Handles connection error.`, func(t *ftt.Test) {
			_, err := client.Get("http+testingfakeprotocol://")
			assert.Loosely(t, err, should.NotBeNil)

			assert.Loosely(t, len(records), should.Equal(2))
			checkRecorded(t, records[0], Request, false, "(?s:GET / HTTP/1.1\r\n(.+:.+\r\n)*)")
			checkRecorded(t, records[1], Response, true)
		})
	})
}
