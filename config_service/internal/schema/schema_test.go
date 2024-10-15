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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/testutil"
)

func TestLookupSchemaURL(t *testing.T) {
	t.Parallel()

	ftt.Run("lookup schema url", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()

		testutil.InjectSelfConfigs(ctx, t, map[string]proto.Message{
			common.SchemaConfigFilePath: &cfgcommonpb.SchemasCfg{
				Schemas: []*cfgcommonpb.SchemasCfg_Schema{
					{
						Name: "project:foo",
						Url:  "https://example.com/foo",
					},
				},
			},
		})

		t.Run("found", func(t *ftt.Test) {
			url, err := lookupSchemaURL(ctx, "project:foo")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, url, should.Equal("https://example.com/foo"))
		})

		t.Run("not found", func(t *ftt.Test) {
			url, err := lookupSchemaURL(ctx, "project:bar")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, url, should.BeEmpty)
		})

	})
}
