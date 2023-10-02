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

package schema

import (
	"testing"

	"google.golang.org/protobuf/proto"

	cfgcommonpb "go.chromium.org/luci/common/proto/config"

	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLookupSchemaURL(t *testing.T) {
	t.Parallel()

	Convey("lookup schema url", t, func() {
		ctx := testutil.SetupContext()

		testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
			common.SchemaConfigFilePath: &cfgcommonpb.SchemasCfg{
				Schemas: []*cfgcommonpb.SchemasCfg_Schema{
					{
						Name: "project:foo",
						Url:  "https://example.com/foo",
					},
				},
			},
		})

		Convey("found", func() {
			url, err := lookupSchemaURL(ctx, "project:foo")
			So(err, ShouldBeNil)
			So(url, ShouldEqual, "https://example.com/foo")
		})

		Convey("not found", func() {
			url, err := lookupSchemaURL(ctx, "project:bar")
			So(err, ShouldBeNil)
			So(url, ShouldBeEmpty)
		})

	})
}
