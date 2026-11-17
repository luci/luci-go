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

package model_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/klauspost/compress/gzip"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/gs"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/config_service/internal/clients"
	"go.chromium.org/luci/config_service/internal/model"
	"go.chromium.org/luci/config_service/testutil"
)

func TestModel(t *testing.T) {
	t.Parallel()

	ftt.Run("model.GetLatestConfigFile", t, func(t *ftt.Test) {
		ctx := memory.UseWithAppID(context.Background(), "dev~app-id")
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		cs := &model.ConfigSet{
			ID:             config.MustServiceSet("service"),
			LatestRevision: model.RevisionInfo{ID: "latest"},
		}
		assert.Loosely(t, datastore.Put(ctx, cs), should.BeNil)

		t.Run("ok", func(t *ftt.Test) {
			var err error
			latest := &model.File{
				Path:     "file",
				Revision: datastore.MakeKey(ctx, model.ConfigSetKind, "services/service", model.RevisionKind, "latest"),
			}
			latest.Content, err = gzipCompress([]byte("latest"))
			assert.NoErr(t, err)
			stale := &model.File{
				Path:     "file",
				Revision: datastore.MakeKey(ctx, model.ConfigSetKind, "services/service", model.RevisionKind, "stale"),
			}
			stale.Content, err = gzipCompress([]byte("stale"))
			assert.NoErr(t, err)
			assert.Loosely(t, datastore.Put(ctx, latest, stale), should.BeNil)
			actual, err := model.GetLatestConfigFile(ctx, config.MustServiceSet("service"), "file")
			assert.NoErr(t, err)
			assert.Loosely(t, actual, should.Resemble(&model.File{
				Path:     "file",
				Revision: datastore.MakeKey(ctx, model.ConfigSetKind, "services/service", model.RevisionKind, "latest"),
				Content:  latest.Content,
			}))
		})

		t.Run("error", func(t *ftt.Test) {
			t.Run("configset not exist", func(t *ftt.Test) {
				_, err := model.GetLatestConfigFile(ctx, config.MustServiceSet("nonexist"), "file")
				assert.ErrIsLike(t, err, `can not find config set entity "services/nonexist"`)
			})

			t.Run("file not exist", func(t *ftt.Test) {
				_, err := model.GetLatestConfigFile(ctx, config.MustServiceSet("service"), "file")
				assert.ErrIsLike(t, err, `can not find file entity "file" from datastore for config set: services/service, revision: latest`)
			})
		})
	})

	ftt.Run("model.GetConfigFileByHash", t, func(t *ftt.Test) {
		ctx := memory.UseWithAppID(context.Background(), "dev~app-id")
		datastore.GetTestable(ctx).Consistent(true)

		t.Run("by content hash", func(t *ftt.Test) {
			content, err := gzipCompress([]byte("content"))
			assert.NoErr(t, err)
			now := clock.Now(ctx).UTC()
			assert.Loosely(t, datastore.Put(ctx, &model.File{
				Path:          "file",
				Revision:      datastore.MakeKey(ctx, model.ConfigSetKind, "services/service", model.RevisionKind, "rev1"),
				ContentSHA256: "hash",
				Content:       content,
				GcsURI:        gs.MakePath("bucket", "object"),
				CreateTime:    datastore.RoundTime(now),
			}, &model.File{
				Path:          "file",
				Revision:      datastore.MakeKey(ctx, model.ConfigSetKind, "services/service", model.RevisionKind, "rev2"),
				ContentSHA256: "hash",
				Content:       content,
				GcsURI:        gs.MakePath("bucket", "object"),
				CreateTime:    datastore.RoundTime(now.Add(1 * time.Minute)),
			}, &model.File{
				Path:          "file",
				Revision:      datastore.MakeKey(ctx, model.ConfigSetKind, "services/another", model.RevisionKind, "rev"),
				ContentSHA256: "hash",
				Content:       content,
				GcsURI:        gs.MakePath("bucket", "object"),
				CreateTime:    datastore.RoundTime(now.Add(2 * time.Minute)),
			}), should.BeNil)

			file, err := model.GetConfigFileByHash(ctx, config.MustServiceSet("service"), "hash")
			assert.NoErr(t, err)
			assert.Loosely(t, file, should.Resemble(&model.File{
				Path:          "file",
				Revision:      datastore.MakeKey(ctx, model.ConfigSetKind, "services/service", model.RevisionKind, "rev2"),
				ContentSHA256: "hash",
				Content:       content,
				GcsURI:        gs.MakePath("bucket", "object"),
				CreateTime:    datastore.RoundTime(now.Add(1 * time.Minute)),
			}))
		})

		t.Run("not found", func(t *ftt.Test) {
			_, err := model.GetConfigFileByHash(ctx, config.MustServiceSet("service"), "hash")
			assert.ErrIsLike(t, err, `can not find matching file entity from datastore with hash "hash"`)
		})
	})
}

func TestGetRawContent(t *testing.T) {
	t.Parallel()
	ftt.Run("GetRawContent", t, func(t *ftt.Test) {
		ctx := memory.UseWithAppID(context.Background(), "dev~app-id")
		ctl := gomock.NewController(t)
		mockGsClient := clients.NewMockGsClient(ctl)
		ctx = clients.WithGsClient(ctx, mockGsClient)
		datastore.GetTestable(ctx).Consistent(true)
		file := &model.File{
			Path:          "file",
			Revision:      datastore.MakeKey(ctx, model.ConfigSetKind, "services/service", model.RevisionKind, "rev"),
			ContentSHA256: "hash",
		}

		t.Run("should read File.Content first", func(t *ftt.Test) {
			content, err := gzipCompress([]byte("raw content"))
			assert.NoErr(t, err)
			file.Content = content
			file.GcsURI = gs.MakePath("test-bucket", "test-object")
			rawContent, err := file.GetRawContent(ctx)
			assert.NoErr(t, err)
			assert.Loosely(t, rawContent, should.Match([]byte("raw content")))
		})

		t.Run("should resolve GcsUri", func(t *ftt.Test) {
			file.GcsURI = gs.MakePath("test-bucket", "test-object")
			sdh := clients.NewMockServiceDataHandle(ctl)
			sdh.EXPECT().Read(gomock.Any()).Return([]byte("raw content"), nil)
			mockGsClient.EXPECT().ServiceDataHandle(gomock.Eq("test-bucket"), gomock.Eq("test-object")).Return(sdh)
			rawContent, err := file.GetRawContent(ctx)
			assert.NoErr(t, err)
			assert.Loosely(t, rawContent, should.Match([]byte("raw content")))

			t.Run("use cache when calling again", func(t *ftt.Test) {
				sdh.EXPECT().Read(gomock.Any()).Return(nil, errors.New("should not be called")).AnyTimes()
				rawContent, err := file.GetRawContent(ctx)
				assert.NoErr(t, err)
				assert.Loosely(t, rawContent, should.Match([]byte("raw content")))
			})
		})

		t.Run("GCS error", func(t *ftt.Test) {
			file.GcsURI = gs.MakePath("test-bucket", "test-object")
			sdh := clients.NewMockServiceDataHandle(ctl)
			sdh.EXPECT().Read(gomock.Any()).Return(nil, errors.New("GCS internal error"))
			mockGsClient.EXPECT().ServiceDataHandle(gomock.Eq("test-bucket"), gomock.Eq("test-object")).Return(sdh)
			rawContent, err := file.GetRawContent(ctx)
			assert.ErrIsLike(t, err, "read from gs://test-bucket/test-object: GCS internal error")
			assert.Loosely(t, rawContent, should.BeNil)
		})

		t.Run("invalid file", func(t *ftt.Test) {
			rawContent, err := file.GetRawContent(ctx)
			assert.ErrIsLike(t, err, "both content and gcs_uri are empty")
			assert.Loosely(t, rawContent, should.BeNil)
		})

		t.Run("empty raw content", func(t *ftt.Test) {
			content, err := gzipCompress([]byte(""))
			assert.NoErr(t, err)
			file.Content = content
			file.GcsURI = gs.MakePath("test-bucket", "test-object")
			rawContent, err := file.GetRawContent(ctx)
			assert.NoErr(t, err)
			assert.Loosely(t, rawContent, should.Match([]byte("")))
		})
	})
}

func TestFileInvalidGzip(t *testing.T) {
	t.Parallel()

	ctx := memory.UseWithAppID(t.Context(), "dev~app-id")
	file := &model.File{
		Path:    "file",
		Content: []byte("not valid gzip"),
	}
	_, err := file.GetRawContent(ctx)
	assert.ErrIsLike(t, err, "gzip: invalid header")
}

func gzipCompress(b []byte) ([]byte, error) {
	buf := &bytes.Buffer{}
	gw := gzip.NewWriter(buf)
	if _, err := gw.Write(b); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func TestLoadSelfConfig(t *testing.T) {
	t.Parallel()

	ctx := testutil.SetupContext()
	projectsCfg := &cfgcommonpb.ProjectsCfg{
		Projects: []*cfgcommonpb.Project{
			{Id: "foo"},
		},
	}
	testutil.InjectSelfConfigs(ctx, t, map[string]proto.Message{
		"projects.cfg": projectsCfg,
	})

	loaded, err := model.LoadSelfConfig[*cfgcommonpb.ProjectsCfg](ctx, "projects.cfg")
	assert.NoErr(t, err)
	assert.Loosely(t, loaded, should.Match(projectsCfg))

	loaded, err = model.LoadSelfConfig[*cfgcommonpb.ProjectsCfg](ctx, "service.cfg")
	assert.ErrIsLike(t, err, "can not find file entity \"service.cfg\" from datastore for config set")

	_, err = model.LoadSelfConfig[*cfgcommonpb.ServicesCfg](ctx, "projects.cfg")
	assert.ErrIsLike(t, err, "failed to unmarshal")
}
