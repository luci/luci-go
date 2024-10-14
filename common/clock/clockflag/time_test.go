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

package clockflag

import (
	"encoding/json"
	"flag"
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestTime(t *testing.T) {
	t.Parallel()

	ftt.Run(`A Time flag`, t, func(t *ftt.Test) {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		var d Time
		fs.Var(&d, "time", "Test time parameter.")

		t.Run(`Parses a 10-second Time from "2015-05-05T23:47:17+00:00".`, func(t *ftt.Test) {
			err := fs.Parse([]string{"-time", "2015-05-05T23:47:17+00:00"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, d.Time().Equal(time.Unix(1430869637, 0)), should.BeTrue)
		})

		t.Run(`Returns an error when parsing "asdf".`, func(t *ftt.Test) {
			err := fs.Parse([]string{"-time", "asdf"})
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run(`When treated as a JSON field`, func(t *ftt.Test) {
			var s struct {
				T Time `json:"time"`
			}

			testJSON := `{"time":"asdf"}`
			t.Run(`Fails to unmarshal from `+testJSON+`.`, func(t *ftt.Test) {
				testJSON := testJSON
				err := json.Unmarshal([]byte(testJSON), &s)
				assert.Loosely(t, err, should.NotBeNil)
			})

			t.Run(`Marshals correctly to RFC3339 time string.`, func(t *ftt.Test) {
				s.T = Time(time.Unix(1430869637, 0))
				testJSON, err := json.Marshal(&s)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, string(testJSON), should.Equal(`{"time":"2015-05-05T23:47:17Z"}`))

				t.Run(`And Unmarshals correctly.`, func(t *ftt.Test) {
					s.T = Time{}
					err := json.Unmarshal([]byte(testJSON), &s)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, s.T.Time().Equal(time.Unix(1430869637, 0)), should.BeTrue)
				})
			})
		})
	})
}
