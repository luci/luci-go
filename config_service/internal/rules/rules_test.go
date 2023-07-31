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

package rules

import (
	"testing"

	"go.chromium.org/luci/common/data/text/pattern"
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAddRules(t *testing.T) {
	t.Parallel()

	Convey("Can derive patterns from rules", t, func() {
		ctx := testutil.SetupContext()

		patterns, err := validation.Rules.ConfigPatterns(ctx)
		So(err, ShouldBeNil)
		So(patterns, ShouldResemble, []*validation.ConfigPattern{
			{
				ConfigSet: pattern.MustParse("exact:services/" + testutil.AppID),
				Path:      pattern.MustParse(common.ACLRegistryFilePath),
			},
			{
				ConfigSet: pattern.MustParse("exact:services/" + testutil.AppID),
				Path:      pattern.MustParse(common.ProjRegistryFilePath),
			},
			{
				ConfigSet: pattern.MustParse("exact:services/" + testutil.AppID),
				Path:      pattern.MustParse(common.ServiceRegistryFilePath),
			},
			{
				ConfigSet: pattern.MustParse("exact:services/" + testutil.AppID),
				Path:      pattern.MustParse(common.ImportConfigFilePath),
			},
			{
				ConfigSet: pattern.MustParse("exact:services/" + testutil.AppID),
				Path:      pattern.MustParse(common.SchemaConfigFilePath),
			},
			{
				ConfigSet: pattern.MustParse(`regex:projects/[^/]+`),
				Path:      pattern.MustParse(common.ProjMetadataFilePath),
			},
			{
				ConfigSet: pattern.MustParse(`regex:.+`),
				Path:      pattern.MustParse(`regex:.+\.json`),
			},
		})
	})
}
