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

package internal

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipd/client/cipd/internal/messages"
)

func TestChecksumCheckingWorks(t *testing.T) {
	msg := messages.TagCache{
		Entries: []*messages.TagCache_Entry{
			{
				Package:    "package",
				Tag:        "tag",
				InstanceId: "instance_id",
			},
		},
	}

	ftt.Run("Works", t, func(c *ftt.Test) {
		buf, err := MarshalWithSHA256(&msg)
		assert.Loosely(c, err, should.BeNil)
		out := messages.TagCache{}
		assert.Loosely(c, UnmarshalWithSHA256(buf, &out), should.BeNil)
		assert.Loosely(c, &out, should.Resemble(&msg))
	})

	ftt.Run("Rejects bad msg", t, func(c *ftt.Test) {
		buf, err := MarshalWithSHA256(&msg)
		assert.Loosely(c, err, should.BeNil)
		buf[10] = 0
		out := messages.TagCache{}
		assert.Loosely(c, UnmarshalWithSHA256(buf, &out), should.NotBeNil)
	})

	ftt.Run("Skips empty sha256", t, func(c *ftt.Test) {
		buf, err := proto.Marshal(&messages.BlobWithSHA256{Blob: []byte{1, 2, 3}})
		assert.Loosely(c, err, should.BeNil)
		out := messages.TagCache{}
		assert.Loosely(c, UnmarshalWithSHA256(buf, &out), should.Equal(ErrUnknownSHA256))
	})
}
