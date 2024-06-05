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

package importscfg

import (
	"context"
	"testing"

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/impl/memory"

	"go.chromium.org/luci/auth_service/api/configspb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestConfigContext(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	importsCfg := &configspb.GroupImporterConfig{
		Tarball: []*configspb.GroupImporterConfig_TarballEntry{
			{
				Url: "some-url",
				OauthScopes: []string{
					"example-oauth",
				},
				Domain: "example.com",
				Systems: []string{
					"groups",
				},
				Groups: []string{
					"committers",
					"testers",
				},
			},
		},
		Plainlist: []*configspb.GroupImporterConfig_PlainlistEntry{
			{
				Url: "plainlistexampleurl",
				OauthScopes: []string{
					"example-oauth-scope",
				},
				Domain: "example.com",
				Group:  "plain-group/example",
			},
		},
		TarballUpload: []*configspb.GroupImporterConfig_TarballUploadEntry{
			{
				Name: "example-tarball.tz",
				AuthorizedUploader: []string{
					"test-service-account@example.com",
				},
				Domain: "example.com",
				Systems: []string{
					"groups",
				},
				Groups: []string{
					"committers",
				},
			},
		},
	}

	Convey("Getting without setting fails", t, func() {
		_, err := Get(ctx)
		So(err, ShouldNotBeNil)

		_, _, err = GetWithMetadata(ctx)
		So(err, ShouldNotBeNil)
	})

	Convey("Testing basic config operations", t, func() {
		So(SetConfig(ctx, importsCfg), ShouldBeNil)
		cfgFromGet, err := Get(ctx)
		So(err, ShouldBeNil)
		So(cfgFromGet, ShouldResembleProto, importsCfg)
	})

	Convey("Testing config operations with metadata", t, func() {
		metadata := &config.Meta{
			Path:     "permissions.cfg",
			Revision: "123abc",
		}
		So(SetConfigWithMetadata(ctx, importsCfg, metadata), ShouldBeNil)
		cfgFromGet, metadataFromGet, err := GetWithMetadata(ctx)
		So(err, ShouldBeNil)
		So(cfgFromGet, ShouldResembleProto, importsCfg)
		So(metadataFromGet, ShouldResembleProto, metadata)
	})
}
