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

	api "go.chromium.org/luci/cipd/api/cipd/v1"
)

func TestGetACLs(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		m := &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{
					Role:       api.Role_WRITER,
					Principals: []string{"a", "b"},
				},
				{
					Role:       api.Role_WRITER,
					Principals: []string{"b"},
				},
				{
					Role:       api.Role_READER,
					Principals: []string{"c"},
				},
			},
		}
		assert.Loosely(t, GetACLs(m), should.Match(map[api.Role]stringset.Set{
			api.Role_READER: {"c": struct{}{}},
			api.Role_WRITER: {"a": struct{}{}, "b": struct{}{}},
		}))
	})
}
