// Copyright 2022 The LUCI Authors.
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

package model

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func testGroupImporterConfig() *GroupImporterConfig {
	return &GroupImporterConfig{
		Kind: "GroupImporterConfig",
		ID:   "config",
		ConfigProto: `
			# Schema for this file:
			# https://luci-config.appspot.com/schemas/services/chrome-infra-auth:imports.cfg
			# See GroupImporterConfig message.

			# Groups pushed by //depot/google3/googleclient/chrome/infra/groups_push_cron/
			#
			# To add new groups, see README.md there.

			tarball_upload {
			name: "test_groups.tar.gz"
			authorized_uploader: "test-push-cron@system.example.com"
			systems: "tst"
			}

			tarball_upload {
			name: "example_groups.tar.gz"
			authorized_uploader: "another-push-cron@system.example.com"
			systems: "examp"
			}
		`,
		ConfigRevision: []byte("some-config-revision"),
		ModifiedBy:     "some-user@example.com",
		ModifiedTS:     testModifiedTS,
	}
}
func TestGroupImporterConfigModel(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())

	Convey("testing GetGroupImporterConfig", t, func() {
		groupCfg := testGroupImporterConfig()

		_, err := GetGroupImporterConfig(ctx)
		So(err, ShouldEqual, datastore.ErrNoSuchEntity)

		So(datastore.Put(ctx, groupCfg), ShouldBeNil)

		actual, err := GetGroupImporterConfig(ctx)
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, groupCfg)
	})
}

func TestLoadGroupFile(t *testing.T) {
	t.Parallel()
	testDomain := "example.com"

	Convey("Testing LoadGroupFile()", t, func() {
		Convey("OK", func() {
			body := strings.Join([]string{"", "b", "a", "a", ""}, "\n")
			aIdent, _ := identity.MakeIdentity(fmt.Sprintf("user:a@%s", testDomain))
			bIdent, _ := identity.MakeIdentity(fmt.Sprintf("user:b@%s", testDomain))

			actual, err := loadGroupFile(body, testDomain)
			So(err, ShouldBeNil)
			So(actual, ShouldResemble, []identity.Identity{
				aIdent,
				bIdent,
			})
		})
		Convey("bad id", func() {
			body := "bad id"
			_, err := loadGroupFile(body, testDomain)
			So(err, ShouldErrLike, `auth: bad value "bad id@example.com" for identity kind "user"`)
		})
	})
}
