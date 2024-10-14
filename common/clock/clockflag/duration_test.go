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

func TestDuration(t *testing.T) {
	t.Parallel()

	ftt.Run(`A Duration flag`, t, func(t *ftt.Test) {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		var d Duration
		fs.Var(&d, "duration", "Test duration parameter.")

		t.Run(`Parses a 10-second Duration from "10s".`, func(t *ftt.Test) {
			err := fs.Parse([]string{"-duration", "10s"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, d, should.Equal(time.Second*10))
			assert.Loosely(t, d.IsZero(), should.BeFalse)
		})

		t.Run(`Returns an error when parsing "10z".`, func(t *ftt.Test) {
			err := fs.Parse([]string{"-duration", "10z"})
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run(`When treated as a JSON field`, func(t *ftt.Test) {
			var s struct {
				D Duration `json:"duration"`
			}

			testJSON := `{"duration":10}`
			t.Run(`Fails to unmarshal from `+testJSON+`.`, func(t *ftt.Test) {
				testJSON := testJSON
				err := json.Unmarshal([]byte(testJSON), &s)
				assert.Loosely(t, err, should.NotBeNil)
			})

			t.Run(`Marshals correctly to duration string.`, func(t *ftt.Test) {
				testDuration := (5 * time.Second) + (2 * time.Minute)
				s.D = Duration(testDuration)
				testJSON, err := json.Marshal(&s)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, string(testJSON), should.Equal(`{"duration":"2m5s"}`))

				t.Run(`And Unmarshals correctly.`, func(t *ftt.Test) {
					s.D = Duration(0)
					err := json.Unmarshal([]byte(testJSON), &s)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, s.D, should.Equal(testDuration))
				})
			})
		})
	})
}
