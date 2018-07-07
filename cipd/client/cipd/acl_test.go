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

package cipd

import (
	"testing"

	api "go.chromium.org/luci/cipd/api/cipd/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGrantRevokeRole(t *testing.T) {
	t.Parallel()

	Convey("Grant role", t, func() {
		m := &api.PrefixMetadata{}

		So(grantRole(m, api.Role_READER, "group:a"), ShouldBeTrue)
		So(grantRole(m, api.Role_READER, "group:b"), ShouldBeTrue)
		So(grantRole(m, api.Role_READER, "group:a"), ShouldBeFalse)
		So(grantRole(m, api.Role_WRITER, "group:a"), ShouldBeTrue)

		So(m, ShouldResemble, &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{Role: api.Role_READER, Principals: []string{"group:a", "group:b"}},
				{Role: api.Role_WRITER, Principals: []string{"group:a"}},
			},
		})
	})

	Convey("Revoke role", t, func() {
		m := &api.PrefixMetadata{
			Acls: []*api.PrefixMetadata_ACL{
				{Role: api.Role_READER, Principals: []string{"group:a", "group:b"}},
				{Role: api.Role_WRITER, Principals: []string{"group:a"}},
			},
		}

		So(revokeRole(m, api.Role_READER, "group:a"), ShouldBeTrue)
		So(revokeRole(m, api.Role_READER, "group:b"), ShouldBeTrue)
		So(revokeRole(m, api.Role_READER, "group:a"), ShouldBeFalse)
		So(revokeRole(m, api.Role_WRITER, "group:a"), ShouldBeTrue)

		So(m, ShouldResemble, &api.PrefixMetadata{})
	})
}
