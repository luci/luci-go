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

	"go.chromium.org/luci/config/validation"
	"google.golang.org/protobuf/encoding/prototext"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"

	configpb "go.chromium.org/luci/resultdb/proto/config"
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

	Convey("invalid config", t, func() {
		cfg := CreatePlaceholderProjectConfig()
		Convey("realm is specified more than once", func() {
			cfg.RealmGcsAllowlist = append(cfg.RealmGcsAllowlist,
				&configpb.RealmGcsAllowList{
					Realm: "a",
				},
				&configpb.RealmGcsAllowList{
					Realm: "a",
				})
			So(validate(cfg), ShouldErrLike, "realm: a is configured more than once")
		})
	})

	Convey("realm GCS allow list", t, func() {
		cfg := CreatePlaceholderProjectConfig()
		So(cfg.RealmGcsAllowlist, ShouldNotBeNil)
		So(len(cfg.RealmGcsAllowlist), ShouldEqual, 1)
		So(len(cfg.RealmGcsAllowlist[0].GcsBucketPrefixes), ShouldEqual, 1)
		So(len(cfg.RealmGcsAllowlist[0].GcsBucketPrefixes[0].AllowedPrefixes), ShouldEqual, 1)
		realmGCSAllowlist := cfg.RealmGcsAllowlist[0]

		Convey("realm", func() {
			Convey("must be specified", func() {
				realmGCSAllowlist.Realm = ""
				So(validate(cfg), ShouldNotBeNil)
			})
			Convey("invalid", func() {
				realmGCSAllowlist.Realm = "a:b"
				So(validate(cfg), ShouldNotBeNil)
			})
			Convey("valid", func() {
				realmGCSAllowlist.Realm = "a"
				So(validate(cfg), ShouldBeNil)
			})
		})

		Convey("GCS bucket prefixes", func() {
			bucketPrefix := realmGCSAllowlist.GcsBucketPrefixes[0]

			Convey("bucket", func() {
				Convey("must be specified", func() {
					bucketPrefix.Bucket = ""
					So(validate(cfg), ShouldErrLike, "empty bucket is not allowed")
				})
				Convey("invalid", func() {
					bucketPrefix.Bucket = "b"
					So(validate(cfg), ShouldErrLike, `invalid bucket: "b"`)
				})
				Convey("valid", func() {
					bucketPrefix.Bucket = "bucket"
					So(validate(cfg), ShouldBeNil)
				})
			})
			Convey("allowed prefixes", func() {
				Convey("invalid len", func() {
					bucketPrefix.AllowedPrefixes[0] = ""
					So(validate(cfg), ShouldErrLike, "prefix: \"\" should have length between 1 and 1024")
				})
				Convey("invalid prefix", func() {
					bucketPrefix.AllowedPrefixes[0] = ".well-known/acme-challenge/a/b/c"
					So(validate(cfg), ShouldErrLike, "prefix: \".well-known/acme-challenge/a/b/c\" is not allowed")
				})
				Convey("invalid value", func() {
					bucketPrefix.AllowedPrefixes[0] = "."
					So(validate(cfg), ShouldErrLike, "prefix: \".\" is not allowed, use '*' as wildcard to allow full access")
				})
				Convey("invalid char", func() {
					bucketPrefix.AllowedPrefixes[0] = "\n"
					So(validate(cfg), ShouldErrLike, "prefix: \"\\n\" contains carriage return or line feed characters, which is not allowed")
				})
				Convey("valid", func() {
					bucketPrefix.AllowedPrefixes[0] = "a/b/c"
					So(validate(cfg), ShouldBeNil)
				})
				Convey("valid wildcard", func() {
					bucketPrefix.AllowedPrefixes[0] = "*"
					So(validate(cfg), ShouldBeNil)
				})
			})
		})
	})
}
