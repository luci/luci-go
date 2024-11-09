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
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/types"
)

func TestFlags(t *testing.T) {
	ftt.Run(`A Flags instance`, t, func(t *ftt.Test) {
		f := Flags{
			Name:        "my/stream",
			ContentType: "foo/bar",
		}

		t.Run(`Converts to LogStreamDescriptor.`, func(t *ftt.Test) {
			assert.Loosely(t, f.Descriptor(), should.Resemble(&logpb.LogStreamDescriptor{
				Name:        "my/stream",
				ContentType: "foo/bar",
				StreamType:  logpb.StreamType_TEXT,
			}))
		})
	})
}

func TestFlagsJSON(t *testing.T) {
	ftt.Run(`A Flags instance`, t, func(t *ftt.Test) {
		f := Flags{}
		t.Run(`Will decode a TEXT stream.`, func(t *ftt.Test) {
			jdesc := `{"name": "my/stream", "contentType": "foo/bar", "type": "text"}`
			assert.Loosely(t, json.Unmarshal([]byte(jdesc), &f), should.BeNil)

			assert.Loosely(t, f.Descriptor(), should.Resemble(&logpb.LogStreamDescriptor{
				Name:        "my/stream",
				ContentType: "foo/bar",
				StreamType:  logpb.StreamType_TEXT,
			}))
		})

		t.Run(`Will fail to decode an invalid stream type.`, func(t *ftt.Test) {
			jdesc := `{"name": "my/stream", "type": "XXX_whatisthis?"}`
			assert.Loosely(t, json.Unmarshal([]byte(jdesc), &f), should.NotBeNil)
		})

		t.Run(`Will decode a BINARY stream.`, func(t *ftt.Test) {
			jdesc := `{"name": "my/stream", "type": "binary"}`
			assert.Loosely(t, json.Unmarshal([]byte(jdesc), &f), should.BeNil)

			assert.Loosely(t, f.Descriptor(), should.Resemble(&logpb.LogStreamDescriptor{
				Name:        "my/stream",
				StreamType:  logpb.StreamType_BINARY,
				ContentType: string(types.ContentTypeBinary),
			}))
		})

		t.Run(`Will decode a DATAGRAM stream.`, func(t *ftt.Test) {
			jdesc := `{"name": "my/stream", "type": "datagram"}`
			assert.Loosely(t, json.Unmarshal([]byte(jdesc), &f), should.BeNil)

			assert.Loosely(t, f.Descriptor(), should.Resemble(&logpb.LogStreamDescriptor{
				Name:        "my/stream",
				StreamType:  logpb.StreamType_DATAGRAM,
				ContentType: string(types.ContentTypeLogdogDatagram),
			}))
		})
	})
}
