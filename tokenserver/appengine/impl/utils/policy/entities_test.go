// Copyright 2017 The LUCI Authors.
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

package policy

import (
	"testing"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestEntitiesWork(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		c := gaetesting.TestingContext()

		hdr, err := getImportedPolicyHeader(c, "policy name")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, hdr, should.BeNil)

		body, err := getImportedPolicyBody(c, "policy name")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, body, should.BeNil)

		assert.Loosely(t, updateImportedPolicy(c, "policy name", "rev", "sha256", []byte("body")), should.BeNil)

		hdr, err = getImportedPolicyHeader(c, "policy name")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, hdr, should.Resemble(&importedPolicyHeader{
			Name:     "policy name",
			Revision: "rev",
			SHA256:   "sha256",
		}))

		body, err = getImportedPolicyBody(c, "policy name")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, body, should.Resemble(&importedPolicyBody{
			Parent:   datastore.KeyForObj(c, hdr),
			Revision: "rev",
			SHA256:   "sha256",
			Data:     []byte("body"),
		}))
	})
}
