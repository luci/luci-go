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

package common

import (
	"net/http"
	"testing"

	"cloud.google.com/go/storage"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/gcloud/gs"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config_service/internal/clients"
	"go.chromium.org/luci/config_service/testutil"

	"github.com/golang/mock/gomock"
	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCommon(t *testing.T) {
	t.Parallel()

	Convey("GitilesURL", t, func() {
		So(GitilesURL(nil), ShouldBeEmpty)
		So(GitilesURL(&cfgcommonpb.GitilesLocation{}), ShouldBeEmpty)

		So(GitilesURL(&cfgcommonpb.GitilesLocation{
			Repo: "https://chromium.googlesource.com/infra/infra",
			Ref:  "refs/heads/main",
			Path: "myfile.cfg",
		}), ShouldEqual, "https://chromium.googlesource.com/infra/infra/+/refs/heads/main/myfile.cfg")

		So(GitilesURL(&cfgcommonpb.GitilesLocation{
			Repo: "https://chromium.googlesource.com/infra/infra",
			Ref:  "refs/heads/main",
		}), ShouldEqual, "https://chromium.googlesource.com/infra/infra/+/refs/heads/main")

		So(GitilesURL(&cfgcommonpb.GitilesLocation{
			Repo: "https://chromium.googlesource.com/infra/infra",
		}), ShouldEqual, "https://chromium.googlesource.com/infra/infra")
	})

	Convey("LoadSelfConfig", t, func() {
		ctx := testutil.SetupContext()
		projectsCfg := &cfgcommonpb.ProjectsCfg{
			Projects: []*cfgcommonpb.Project{
				{Id: "foo"},
			},
		}
		testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
			"projects.cfg": projectsCfg,
		})

		loaded := &cfgcommonpb.ProjectsCfg{}
		So(LoadSelfConfig[*cfgcommonpb.ProjectsCfg](ctx, "projects.cfg", loaded), ShouldBeNil)
		So(loaded, ShouldResembleProto, projectsCfg)

		So(LoadSelfConfig[*cfgcommonpb.ProjectsCfg](ctx, "service.cfg", loaded), ShouldErrLike, "can not find file entity \"service.cfg\" from datastore for config set")

		So(LoadSelfConfig[*cfgcommonpb.ServicesCfg](ctx, "projects.cfg", &cfgcommonpb.ServicesCfg{}), ShouldErrLike, "failed to unmarshal")
	})

	Convey("CreateSignedURLs", t, func() {
		ctx := testutil.SetupContext()
		ctl := gomock.NewController(t)
		mockGsClient := clients.NewMockGsClient(ctl)
		paths := []gs.Path{
			gs.MakePath("test-bucket-1", "test-object-1"),
			gs.MakePath("test-bucket-2", "test-object-2"),
		}

		var recordedOpts *storage.SignedURLOptions
		mockGsClient.EXPECT().SignedURL(
			gomock.Eq("test-bucket-1"),
			gomock.Eq("test-object-1"),
			gomock.AssignableToTypeOf(recordedOpts),
		).DoAndReturn(
			func(_, _ string, opts *storage.SignedURLOptions) (string, error) {
				recordedOpts = opts
				return "signed-url-1", nil
			},
		)
		mockGsClient.EXPECT().SignedURL(
			gomock.Eq("test-bucket-2"),
			gomock.Eq("test-object-2"),
			gomock.Any(),
		).Return("signed-url-2", nil)

		urls, err := CreateSignedURLs(ctx, mockGsClient, paths, http.MethodGet, map[string]string{
			"Custom-Header":         "Value",
			"Another-Custom-Header": "Another-Value",
		})
		So(err, ShouldBeNil)
		So(urls, ShouldResemble, []string{"signed-url-1", "signed-url-2"})
		So(recordedOpts.GoogleAccessID, ShouldEqual, testutil.ServiceAccount)
		So(recordedOpts.Scheme, ShouldEqual, storage.SigningSchemeV4)
		So(recordedOpts.Method, ShouldEqual, http.MethodGet)
		So(recordedOpts.Expires, ShouldEqual, clock.Now(ctx).Add(signedURLExpireDur))
		So(recordedOpts.Headers, ShouldResemble, []string{"Another-Custom-Header:Another-Value", "Custom-Header:Value"})
	})
}
