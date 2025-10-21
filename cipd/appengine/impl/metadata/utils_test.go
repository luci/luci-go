// Copyright 2018 The LUCI Authors.
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

package metadata

import (
	"testing"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
)

func TestGetACLs(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		m := &repopb.PrefixMetadata{
			Acls: []*repopb.PrefixMetadata_ACL{
				{
					Role:       repopb.Role_WRITER,
					Principals: []string{"a", "b"},
				},
				{
					Role:       repopb.Role_WRITER,
					Principals: []string{"b"},
				},
				{
					Role:       repopb.Role_READER,
					Principals: []string{"c"},
				},
			},
		}
		assert.Loosely(t, GetACLs(m), should.Match(map[repopb.Role]stringset.Set{
			repopb.Role_READER: {"c": struct{}{}},
			repopb.Role_WRITER: {"a": struct{}{}, "b": struct{}{}},
		}))
	})
}
