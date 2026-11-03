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

package dsstore

import (
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	cfgcachepb "go.chromium.org/luci/server/cfgcache/proto"
)

func TestContentMetaProperty(t *testing.T) {
	t.Parallel()

	t.Run(`round trip`, func(t *testing.T) {
		t.Parallel()

		prop := contentMetaProperty{
			PipelineID: "stuff",
			Meta: &cfgcachepb.Meta{
				InputHash:        "coolbeans",
				InputSize:        20,
				TransformVersion: 10,
				TransformedSize:  1000,
			},
		}

		// NOTE: We only care about determinism of the prefix portion of the
		// property (before the `|`) and *technically* proto.Marshal is
		// non-deterministic - so just assert the prefix we care about, and make
		// sure we can round trip the full encoded string.
		encoded := prop.String()
		assert.Loosely(t, encoded, should.HavePrefix("c3R1ZmY|"))

		dec := contentMetaProperty{}
		assert.NoErr(t, dec.FromProperty(datastore.MkProperty(encoded)))

		assert.That(t, dec, should.Match(prop))
	})
}
