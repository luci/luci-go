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

package config

import (
	"context"
	"os"
	"testing"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/config/validation"

	configpb "go.chromium.org/luci/resultdb/proto/config"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestProjectConfigValidator(t *testing.T) {
	t.Parallel()

	validate := func(cfg *configpb.ProjectConfig) error {
		c := validation.Context{Context: context.Background()}
		validateProjectConfig(&c, cfg)
		return c.Finalize()
	}

	Convey("config template is valid", t, func() {
		content, err := os.ReadFile(
			"../../configs/projects/chromeos/luci-resultdb-dev-template.cfg",
		)
		So(err, ShouldBeNil)
		cfg := &configpb.ProjectConfig{}
		So(prototext.Unmarshal(content, cfg), ShouldBeNil)
		So(validate(cfg), ShouldBeNil)
	})

	Convey("valid config is valid", t, func() {
		cfg := CreatePlaceholderProjectConfig()
		So(validate(cfg), ShouldBeNil)
	})

	Convey("GCS allow list", t, func() {
		cfg := CreatePlaceholderProjectConfig()
		So(cfg.GcsAllowList, ShouldNotBeNil)
		So(len(cfg.GcsAllowList), ShouldEqual, 1)
		So(len(cfg.GcsAllowList[0].Buckets), ShouldEqual, 1)
		gcsAllowList := cfg.GcsAllowList[0]

		Convey("users", func() {
			Convey("must be specified", func() {
				gcsAllowList.Users = []string{}
				So(validate(cfg), ShouldNotBeNil)
			})
			Convey("must be non-empty", func() {
				gcsAllowList.Users = []string{""}
				So(validate(cfg), ShouldNotBeNil)
			})
			Convey("invalid", func() {
				gcsAllowList.Users = []string{"a:b"}
				So(validate(cfg), ShouldNotBeNil)
			})
			Convey("valid", func() {
				gcsAllowList.Users = []string{"user:test@test.com"}
				So(validate(cfg), ShouldBeNil)
			})
			Convey("multiple", func() {
				gcsAllowList.Users = []string{"user:test@test.com", "user:test2@test.com"}
				So(validate(cfg), ShouldBeNil)
			})
		})

		Convey("GCS buckets", func() {
			Convey("bucket", func() {
				Convey("must be specified", func() {
					gcsAllowList.Buckets[0] = ""
					So(validate(cfg), ShouldErrLike, "empty bucket is not allowed")
				})
				Convey("invalid", func() {
					gcsAllowList.Buckets[0] = "b"
					So(validate(cfg), ShouldErrLike, `invalid bucket: "b"`)
				})
				Convey("valid", func() {
					gcsAllowList.Buckets[0] = "bucket"
					So(validate(cfg), ShouldBeNil)
				})
			})
		})
	})
}

func TestServiceConfigValidator(t *testing.T) {
	t.Parallel()

	validate := func(cfg *configpb.Config) error {
		c := validation.Context{Context: context.Background()}
		validateServiceConfig(&c, cfg)
		return c.Finalize()
	}

	Convey("config template is valid", t, func() {
		content, err := os.ReadFile(
			"../../configs/service/template.cfg",
		)
		So(err, ShouldBeNil)
		cfg := &configpb.Config{}
		So(prototext.Unmarshal(content, cfg), ShouldBeNil)
		So(validate(cfg), ShouldBeNil)
	})

	Convey("valid config is valid", t, func() {
		cfg := CreatePlaceHolderServiceConfig()
		So(validate(cfg), ShouldBeNil)
	})

	Convey("bq artifact export config", t, func() {
		cfg := CreatePlaceHolderServiceConfig()
		Convey("is nil", func() {
			cfg.BqArtifactExportConfig = nil
			So(validate(cfg), ShouldNotBeNil)
		})

		Convey("percentage smaller than 0", func() {
			cfg.BqArtifactExportConfig = &configpb.BqArtifactExportConfig{
				ExportPercent: -1,
			}
			So(validate(cfg), ShouldNotBeNil)
		})

		Convey("percentage bigger than 100", func() {
			cfg.BqArtifactExportConfig = &configpb.BqArtifactExportConfig{
				ExportPercent: 101,
			}
			So(validate(cfg), ShouldNotBeNil)
		})
	})
}
