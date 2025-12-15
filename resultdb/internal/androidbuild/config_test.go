// Copyright 2025 The LUCI Authors.
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

package androidbuild

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	configpb "go.chromium.org/luci/resultdb/proto/config"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("Config", t, func(t *ftt.Test) {
		cfg := &configpb.AndroidBuild{
			DataRealmPattern: `^prod|test$`,
			DataRealms: map[string]*configpb.AndroidBuild_ByDataRealmConfig{
				"prod": {
					FullBuildUrlTemplate: "https://android-build.googleplex.com/build_explorer/build_details/${build_id}/${build_target}/",
				},
			},
		}
		c, err := NewConfig(cfg)
		assert.Loosely(t, err, should.BeNil)

		t.Run("With default config", func(t *ftt.Test) {
			_, err := NewConfig(nil)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("ValidateBuildDescriptor", func(t *ftt.Test) {
			descr := &pb.AndroidBuildDescriptor{
				DataRealm:   "test",
				Branch:      "git_main",
				BuildTarget: "cf_x86_phone-userdebug",
				BuildId:     "P81983588",
			}
			t.Run("Valid", func(t *ftt.Test) {
				err := c.ValidateBuildDescriptor(descr)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("Invalid data_realm", func(t *ftt.Test) {
				descr.DataRealm = "test-invalid"
				err := c.ValidateBuildDescriptor(descr)
				assert.Loosely(t, err, should.ErrLike(`data_realm: does not match pattern "^prod|test$"`))
			})
		})

		t.Run("ValidateSubmittedBuild", func(t *ftt.Test) {
			descr := &pb.SubmittedAndroidBuild{
				DataRealm: "test",
				Branch:    "git_main",
				BuildId:   12345678,
			}
			t.Run("Valid", func(t *ftt.Test) {
				err := c.ValidateSubmittedBuild(descr)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("Invalid data_realm", func(t *ftt.Test) {
				descr.DataRealm = "test-invalid"
				err := c.ValidateSubmittedBuild(descr)
				assert.Loosely(t, err, should.ErrLike(`data_realm: does not match pattern "^prod|test$"`))
			})
		})

		t.Run("GenerateBuildDescriptorURL", func(t *ftt.Test) {
			t.Run("Config not present", func(t *ftt.Test) {
				url := c.GenerateBuildDescriptorURL(&pb.AndroidBuildDescriptor{
					DataRealm:   "test",
					Branch:      "git_main",
					BuildTarget: "cf_x86_phone-userdebug",
					BuildId:     "P81983588",
				})
				assert.Loosely(t, url, should.Equal(""))
			})

			t.Run("Config present for data realm", func(t *ftt.Test) {
				url := c.GenerateBuildDescriptorURL(&pb.AndroidBuildDescriptor{
					DataRealm:   "prod",
					Branch:      "git_main",
					BuildTarget: "cf_x86_phone-userdebug",
					BuildId:     "P81983588",
				})
				assert.Loosely(t, url, should.Equal("https://android-build.googleplex.com/build_explorer/build_details/P81983588/cf_x86_phone-userdebug/"))
			})
		})
	})
}
