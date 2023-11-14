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
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/config_service/internal/clients"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestModel(t *testing.T) {
	t.Parallel()

	Convey("GetLatestConfigFile", t, func() {
		ctx := memory.UseWithAppID(context.Background(), "dev~app-id")
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		cs := &ConfigSet{
			ID:             config.MustServiceSet("service"),
			LatestRevision: RevisionInfo{ID: "latest"},
		}
		So(datastore.Put(ctx, cs), ShouldBeNil)

		Convey("ok", func() {
			var err error
			latest := &File{
				Path:     "file",
				Revision: datastore.MakeKey(ctx, ConfigSetKind, "services/service", RevisionKind, "latest"),
			}
			latest.Content, err = gzipCompress([]byte("latest"))
			So(err, ShouldBeNil)
			stale := &File{
				Path:     "file",
				Revision: datastore.MakeKey(ctx, ConfigSetKind, "services/service", RevisionKind, "stale"),
			}
			stale.Content, err = gzipCompress([]byte("stale"))
			So(err, ShouldBeNil)
			So(datastore.Put(ctx, latest, stale), ShouldBeNil)
			actual, err := GetLatestConfigFile(ctx, config.MustServiceSet("service"), "file")
			So(err, ShouldBeNil)
			So(actual, ShouldResemble, &File{
				Path:     "file",
				Revision: datastore.MakeKey(ctx, ConfigSetKind, "services/service", RevisionKind, "latest"),
				Content:  latest.Content,
			})
		})

		Convey("error", func() {
			Convey("configset not exist", func() {
				_, err := GetLatestConfigFile(ctx, config.MustServiceSet("nonexist"), "file")
				So(err, ShouldErrLike, `can not find config set entity "services/nonexist"`)
			})

			Convey("file not exist", func() {
				_, err := GetLatestConfigFile(ctx, config.MustServiceSet("service"), "file")
				So(err, ShouldErrLike, `can not find file entity "file" from datastore for config set: services/service, revision: latest`)
			})
		})
	})

	Convey("GetConfigFileByHash", t, func() {
		ctx := memory.UseWithAppID(context.Background(), "dev~app-id")
		datastore.GetTestable(ctx).Consistent(true)

		Convey("by content hash", func() {
			content, err := gzipCompress([]byte("content"))
			So(err, ShouldBeNil)
			now := clock.Now(ctx).UTC()
			So(datastore.Put(ctx, &File{
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
			}), ShouldBeNil)

			file, err := GetConfigFileByHash(ctx, config.MustServiceSet("service"), "hash")
			So(err, ShouldBeNil)
			So(file, ShouldResemble, &File{
				Path:          "file",
				Revision:      datastore.MakeKey(ctx, ConfigSetKind, "services/service", RevisionKind, "rev2"),
				ContentSHA256: "hash",
				Content:       content,
				GcsURI:        gs.MakePath("bucket", "object"),
				CreateTime:    datastore.RoundTime(now.Add(1 * time.Minute)),
			})
		})

		Convey("not found", func() {
			_, err := GetConfigFileByHash(ctx, config.MustServiceSet("service"), "hash")
			So(err, ShouldErrLike, `can not find matching file entity from datastore with hash "hash"`)
		})
	})
}

func TestGetRawContent(t *testing.T) {
	t.Parallel()
	Convey("GetRawContent", t, func() {
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

		Convey("should read File.Content first", func() {
			content, err := gzipCompress([]byte("raw content"))
			So(err, ShouldBeNil)
			file.Content = content
			file.GcsURI = gs.MakePath("test-bucket", "test-object")
			rawContent, err := file.GetRawContent(ctx)
			So(err, ShouldBeNil)
			So(rawContent, ShouldEqual, []byte("raw content"))
			So(file.rawContent, ShouldEqual, []byte("raw content"))
		})

		Convey("should resolve GcsUri", func() {
			content, err := gzipCompress([]byte("raw content"))
			So(err, ShouldBeNil)
			file.GcsURI = gs.MakePath("test-bucket", "test-object")
			mockGsClient.EXPECT().Read(gomock.Any(), gomock.Eq("test-bucket"), gomock.Eq("test-object"), false).Return(content, nil)
			rawContent, err := file.GetRawContent(ctx)
			So(err, ShouldBeNil)
			So(rawContent, ShouldEqual, []byte("raw content"))
			So(file.rawContent, ShouldEqual, []byte("raw content"))

			Convey("use cache when calling agin", func() {
				mockGsClient.EXPECT().Read(gomock.Any(), gomock.Eq("test-bucket"), gomock.Eq("test-object"), false).Return(nil, errors.New("should not be called")).AnyTimes()
				rawContent, err := file.GetRawContent(ctx)
				So(err, ShouldBeNil)
				So(rawContent, ShouldEqual, []byte("raw content"))
			})
		})

		Convey("GCS error", func() {
			file.GcsURI = gs.MakePath("test-bucket", "test-object")
			mockGsClient.EXPECT().Read(gomock.Any(), gomock.Eq("test-bucket"), gomock.Eq("test-object"), false).Return(nil, errors.New("GCS internal error"))
			rawContent, err := file.GetRawContent(ctx)
			So(err, ShouldErrLike, "failed to read from gs://test-bucket/test-object: GCS internal error")
			So(rawContent, ShouldBeNil)
		})

		Convey("invalid file", func() {
			rawContent, err := file.GetRawContent(ctx)
			So(err, ShouldErrLike, "both content and gcs_uri are empty")
			So(rawContent, ShouldBeNil)
		})

		Convey("empty raw content", func() {
			content, err := gzipCompress([]byte(""))
			So(err, ShouldBeNil)
			file.Content = content
			file.GcsURI = gs.MakePath("test-bucket", "test-object")
			rawContent, err := file.GetRawContent(ctx)
			So(err, ShouldBeNil)
			So(rawContent, ShouldEqual, []byte(""))
			So(file.rawContent, ShouldEqual, []byte(""))
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
