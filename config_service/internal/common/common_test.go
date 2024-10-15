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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config_service/internal/clients"
	"go.chromium.org/luci/config_service/testutil"

	"github.com/golang/mock/gomock"
)

func TestCommon(t *testing.T) {
	t.Parallel()

	ftt.Run("GitilesURL", t, func(t *ftt.Test) {
		assert.Loosely(t, GitilesURL(nil), should.BeEmpty)
		assert.Loosely(t, GitilesURL(&cfgcommonpb.GitilesLocation{}), should.BeEmpty)

		assert.Loosely(t, GitilesURL(&cfgcommonpb.GitilesLocation{
			Repo: "https://chromium.googlesource.com/infra/infra",
			Ref:  "refs/heads/main",
			Path: "myfile.cfg",
		}), should.Equal("https://chromium.googlesource.com/infra/infra/+/refs/heads/main/myfile.cfg"))

		assert.Loosely(t, GitilesURL(&cfgcommonpb.GitilesLocation{
			Repo: "https://chromium.googlesource.com/infra/infra",
			Ref:  "refs/heads/main",
		}), should.Equal("https://chromium.googlesource.com/infra/infra/+/refs/heads/main"))

		assert.Loosely(t, GitilesURL(&cfgcommonpb.GitilesLocation{
			Repo: "https://chromium.googlesource.com/infra/infra",
		}), should.Equal("https://chromium.googlesource.com/infra/infra"))
	})

	ftt.Run("LoadSelfConfig", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()
		projectsCfg := &cfgcommonpb.ProjectsCfg{
			Projects: []*cfgcommonpb.Project{
				{Id: "foo"},
			},
		}
		testutil.InjectSelfConfigs(ctx, t, map[string]proto.Message{
			"projects.cfg": projectsCfg,
		})

		loaded := &cfgcommonpb.ProjectsCfg{}
		assert.Loosely(t, LoadSelfConfig[*cfgcommonpb.ProjectsCfg](ctx, "projects.cfg", loaded), should.BeNil)
		assert.Loosely(t, loaded, should.Resemble(projectsCfg))

		assert.Loosely(t, LoadSelfConfig[*cfgcommonpb.ProjectsCfg](ctx, "service.cfg", loaded), should.ErrLike("can not find file entity \"service.cfg\" from datastore for config set"))

		assert.Loosely(t, LoadSelfConfig[*cfgcommonpb.ServicesCfg](ctx, "projects.cfg", &cfgcommonpb.ServicesCfg{}), should.ErrLike("failed to unmarshal"))
	})

	ftt.Run("CreateSignedURLs", t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, urls, should.Resemble([]string{"signed-url-1", "signed-url-2"}))
		assert.Loosely(t, recordedOpts.GoogleAccessID, should.Equal(testutil.ServiceAccount))
		assert.Loosely(t, recordedOpts.Scheme, should.Equal(storage.SigningSchemeV4))
		assert.Loosely(t, recordedOpts.Method, should.Equal(http.MethodGet))
		assert.Loosely(t, recordedOpts.Expires, should.Match(clock.Now(ctx).Add(signedURLExpireDur)))
		assert.Loosely(t, recordedOpts.Headers, should.Resemble([]string{"Another-Custom-Header:Another-Value", "Custom-Header:Value"}))
	})
}
