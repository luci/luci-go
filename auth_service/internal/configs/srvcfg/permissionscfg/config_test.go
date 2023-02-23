// Copyright 2023 The LUCI Authors.
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

package permissionscfg

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/auth_service/api/configspb"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/gae/impl/memory"
)

func TestConfigContext(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	permissionsCfg := &configspb.PermissionsConfig{
		Role: []*configspb.PermissionsConfig_Role{
			{
				Name: "role/test.datastore.reader",
				Permissions: []string{
					"test.datastore.get",
					"test.datastore.list",
				},
			},
			{
				Name: "role/test.datastore.writer",
				Permissions: []string{
					"test.datastore.write",
					"test.datastore.create",
				},
			},
			{
				Name: "role/test.datastore.admin",
				Permissions: []string{
					"test.datastore.delete",
				},
				Includes: []string{
					"role/test.datastore.reader",
					"role/test.datastore.writer",
				},
			},
		},
	}
	Convey("Testing basic config operations", t, func() {
		So(SetConfig(ctx, permissionsCfg), ShouldBeNil)
		cfgFromGet, err := Get(ctx)
		So(err, ShouldBeNil)
		So(cfgFromGet, ShouldResembleProto, permissionsCfg)
	})
}
