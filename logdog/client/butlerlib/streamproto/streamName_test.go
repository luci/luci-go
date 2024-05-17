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

package streamproto

import (
	"encoding/json"
	"flag"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestStreamName(t *testing.T) {
	ftt.Run(`A StreamNameFlag`, t, func(t *ftt.Test) {
		sn := StreamNameFlag("")

		t.Run(`When attached to a flag.`, func(t *ftt.Test) {
			fs := flag.FlagSet{}
			fs.Var(&sn, "name", "Test stream name.")

			t.Run(`Will successfully parse a valid stream name.`, func(t *ftt.Test) {
				err := fs.Parse([]string{"-name", "test/stream/name"})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, sn, should.Equal(StreamNameFlag("test/stream/name")))
			})

			t.Run(`Will fail to parse an invalid stream name.`, func(t *ftt.Test) {
				err := fs.Parse([]string{"-name", "test;stream;name"})
				assert.Loosely(t, err, should.NotBeNil)
			})
		})

		t.Run(`With valid value "test/stream/name"`, func(t *ftt.Test) {
			t.Run(`Will marshal into a JSON string.`, func(t *ftt.Test) {
				sn = "test/stream/name"
				d, err := json.Marshal(&sn)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, string(d), should.Resemble(`"test/stream/name"`))

				t.Run(`And successfully unmarshal.`, func(t *ftt.Test) {
					sn = ""
					err := json.Unmarshal(d, &sn)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, sn, should.Equal(StreamNameFlag("test/stream/name")))
				})
			})
		})

		t.Run(`JSON with an invalid value "test;stream;name" will fail to Unmarshal.`, func(t *ftt.Test) {
			sn := StreamNameFlag("")
			err := json.Unmarshal([]byte(`"test;stream;name"`), &sn)
			assert.Loosely(t, err, should.NotBeNil)
		})
	})
}
