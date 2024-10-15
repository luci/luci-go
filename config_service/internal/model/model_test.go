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

package model

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/klauspost/compress/gzip"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/config_service/internal/clients"
)

func TestModel(t *testing.T) {
	t.Parallel()

	ftt.Run("GetLatestConfigFile", t, func(t *ftt.Test) {
		ctx := memory.UseWithAppID(context.Background(), "dev~app-id")
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		cs := &ConfigSet{
			ID:             config.MustServiceSet("service"),
			LatestRevision: RevisionInfo{ID: "latest"},
		}
		assert.Loosely(t, datastore.Put(ctx, cs), should.BeNil)

		t.Run("ok", func(t *ftt.Test) {
			var err error
			latest := &File{
				Path:     "file",
				Revision: datastore.MakeKey(ctx, ConfigSetKind, "services/service", RevisionKind, "latest"),
			}
			latest.Content, err = gzipCompress([]byte("latest"))
			assert.Loosely(t, err, should.BeNil)
			stale := &File{
				Path:     "file",
				Revision: datastore.MakeKey(ctx, ConfigSetKind, "services/service", RevisionKind, "stale"),
			}
			stale.Content, err = gzipCompress([]byte("stale"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, latest, stale), should.BeNil)
			actual, err := GetLatestConfigFile(ctx, config.MustServiceSet("service"), "file")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Resemble(&File{
				Path:     "file",
				Revision: datastore.MakeKey(ctx, ConfigSetKind, "services/service", RevisionKind, "latest"),
				Content:  latest.Content,
			}))
		})

		t.Run("error", func(t *ftt.Test) {
			t.Run("configset not exist", func(t *ftt.Test) {
				_, err := GetLatestConfigFile(ctx, config.MustServiceSet("nonexist"), "file")
				assert.Loosely(t, err, should.ErrLike(`can not find config set entity "services/nonexist"`))
			})

			t.Run("file not exist", func(t *ftt.Test) {
				_, err := GetLatestConfigFile(ctx, config.MustServiceSet("service"), "file")
				assert.Loosely(t, err, should.ErrLike(`can not find file entity "file" from datastore for config set: services/service, revision: latest`))
			})
		})
	})

	ftt.Run("GetConfigFileByHash", t, func(t *ftt.Test) {
		ctx := memory.UseWithAppID(context.Background(), "dev~app-id")
		datastore.GetTestable(ctx).Consistent(true)

		t.Run("by content hash", func(t *ftt.Test) {
			content, err := gzipCompress([]byte("content"))
			assert.Loosely(t, err, should.BeNil)
			now := clock.Now(ctx).UTC()
			assert.Loosely(t, datastore.Put(ctx, &File{
				Path:          "file",
				Revision:      datastore.MakeKey(ctx, ConfigSetKind, "services/service", RevisionKind, "rev1"),
				ContentSHA256: "hash",
				Content:       content,
				GcsURI:        gs.MakePath("bucket", "object"),
				CreateTime:    datastore.RoundTime(now),
			}, &File{
				Path:          "file",
				Revision:      datastore.MakeKey(ctx, ConfigSetKind, "services/service", RevisionKind, "rev2"),
				ContentSHA256: "hash",
				Content:       content,
				GcsURI:        gs.MakePath("bucket", "object"),
				CreateTime:    datastore.RoundTime(now.Add(1 * time.Minute)),
			}, &File{
				Path:          "file",
				Revision:      datastore.MakeKey(ctx, ConfigSetKind, "services/another", RevisionKind, "rev"),
				ContentSHA256: "hash",
				Content:       content,
				GcsURI:        gs.MakePath("bucket", "object"),
				CreateTime:    datastore.RoundTime(now.Add(2 * time.Minute)),
			}), should.BeNil)

			file, err := GetConfigFileByHash(ctx, config.MustServiceSet("service"), "hash")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, file, should.Resemble(&File{
				Path:          "file",
				Revision:      datastore.MakeKey(ctx, ConfigSetKind, "services/service", RevisionKind, "rev2"),
				ContentSHA256: "hash",
				Content:       content,
				GcsURI:        gs.MakePath("bucket", "object"),
				CreateTime:    datastore.RoundTime(now.Add(1 * time.Minute)),
			}))
		})

		t.Run("not found", func(t *ftt.Test) {
			_, err := GetConfigFileByHash(ctx, config.MustServiceSet("service"), "hash")
			assert.Loosely(t, err, should.ErrLike(`can not find matching file entity from datastore with hash "hash"`))
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
		file := &File{
			Path:          "file",
			Revision:      datastore.MakeKey(ctx, ConfigSetKind, "services/service", RevisionKind, "rev"),
			ContentSHA256: "hash",
		}

		t.Run("should read File.Content first", func(t *ftt.Test) {
			content, err := gzipCompress([]byte("raw content"))
			assert.Loosely(t, err, should.BeNil)
			file.Content = content
			file.GcsURI = gs.MakePath("test-bucket", "test-object")
			rawContent, err := file.GetRawContent(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rawContent, should.Match([]byte("raw content")))
			assert.Loosely(t, file.rawContent, should.Match([]byte("raw content")))
		})

		t.Run("should resolve GcsUri", func(t *ftt.Test) {
			content, err := gzipCompress([]byte("raw content"))
			assert.Loosely(t, err, should.BeNil)
			file.GcsURI = gs.MakePath("test-bucket", "test-object")
			mockGsClient.EXPECT().Read(gomock.Any(), gomock.Eq("test-bucket"), gomock.Eq("test-object"), false).Return(content, nil)
			rawContent, err := file.GetRawContent(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rawContent, should.Match([]byte("raw content")))
			assert.Loosely(t, file.rawContent, should.Match([]byte("raw content")))

			t.Run("use cache when calling agin", func(t *ftt.Test) {
				mockGsClient.EXPECT().Read(gomock.Any(), gomock.Eq("test-bucket"), gomock.Eq("test-object"), false).Return(nil, errors.New("should not be called")).AnyTimes()
				rawContent, err := file.GetRawContent(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rawContent, should.Match([]byte("raw content")))
			})
		})

		t.Run("GCS error", func(t *ftt.Test) {
			file.GcsURI = gs.MakePath("test-bucket", "test-object")
			mockGsClient.EXPECT().Read(gomock.Any(), gomock.Eq("test-bucket"), gomock.Eq("test-object"), false).Return(nil, errors.New("GCS internal error"))
			rawContent, err := file.GetRawContent(ctx)
			assert.Loosely(t, err, should.ErrLike("failed to read from gs://test-bucket/test-object: GCS internal error"))
			assert.Loosely(t, rawContent, should.BeNil)
		})

		t.Run("invalid file", func(t *ftt.Test) {
			rawContent, err := file.GetRawContent(ctx)
			assert.Loosely(t, err, should.ErrLike("both content and gcs_uri are empty"))
			assert.Loosely(t, rawContent, should.BeNil)
		})

		t.Run("empty raw content", func(t *ftt.Test) {
			content, err := gzipCompress([]byte(""))
			assert.Loosely(t, err, should.BeNil)
			file.Content = content
			file.GcsURI = gs.MakePath("test-bucket", "test-object")
			rawContent, err := file.GetRawContent(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rawContent, should.Match([]byte("")))
			assert.Loosely(t, file.rawContent, should.Match([]byte("")))
		})
	})
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
