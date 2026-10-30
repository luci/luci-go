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

	"go.chromium.org/luci/server/cfgcache"
)

func TestContentMetaProperty(t *testing.T) {
	t.Parallel()

	t.Run(`round trip`, func(t *testing.T) {
		t.Parallel()

		prop := contentMetaProperty{
			PipelineID: "stuff",
			Meta: cfgcache.Meta{
				InputHash:        "coolbeans",
				TransformVersion: 10,
				ContentSize:      1000,
			},
		}

		propEnc := "c3R1ZmY|coolbeans|10|1000"

		assert.That(t, prop.String(), should.Equal(propEnc))

		dec := contentMetaProperty{}
		assert.NoErr(t, dec.FromProperty(datastore.MkProperty(propEnc)))

		assert.That(t, dec, should.Match(prop))
	})
}
