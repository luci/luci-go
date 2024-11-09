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
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/logdog/api/logpb"
)

func TestStreamType(t *testing.T) {
	ftt.Run(`A StreamType flag`, t, func(t *ftt.Test) {
		value := StreamType(0)

		fs := flag.NewFlagSet("Testing", flag.ContinueOnError)
		fs.Var(&value, "stream-type", "StreamType test.")

		t.Run(`Can be loaded as a flag.`, func(t *ftt.Test) {
			err := fs.Parse([]string{"-stream-type", "datagram"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, value, should.Equal(logpb.StreamType_DATAGRAM))
		})

		t.Run(`Will unmmarshal from JSON.`, func(t *ftt.Test) {
			var s struct {
				Value StreamType `json:"value"`
			}

			err := json.Unmarshal([]byte(`{"value": "text"}`), &s)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, s.Value, should.Equal(logpb.StreamType_TEXT))
		})

		t.Run(`Will marshal to JSON.`, func(t *ftt.Test) {
			var s struct {
				Value StreamType `json:"value"`
			}
			s.Value = StreamType(logpb.StreamType_BINARY)

			v, err := json.Marshal(&s)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(v), should.Match(`{"value":"binary"}`))
		})

		for _, st := range []logpb.StreamType{
			logpb.StreamType_TEXT,
			logpb.StreamType_BINARY,
			logpb.StreamType_DATAGRAM,
		} {
			t.Run(fmt.Sprintf(`Stream type [%s] has a default content type.`, st), func(t *ftt.Test) {
				sst := StreamType(st)
				assert.Loosely(t, sst.DefaultContentType(), should.NotEqual(""))
			})
		}
	})
}
