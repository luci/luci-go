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
	"compress/gzip"
	"context"
	"testing"

	"github.com/golang/mock/gomock"

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
			actual, err := GetLatestConfigFile(ctx, config.MustServiceSet("service"), "file", false)
			So(err, ShouldBeNil)
			So(actual, ShouldResemble, &File{
				Path:     "file",
				Revision: datastore.MakeKey(ctx, ConfigSetKind, "services/service", RevisionKind, "latest"),
				Content:  []byte("latest"),
			})
		})

		Convey("error", func() {
			Convey("configset not exist", func() {
				_, err := GetLatestConfigFile(ctx, config.MustServiceSet("nonexist"), "file", false)
				So(err, ShouldErrLike, `can not find config set entity "services/nonexist"`)
			})

			Convey("file not exist", func() {
				_, err := GetLatestConfigFile(ctx, config.MustServiceSet("service"), "file", false)
				So(err, ShouldErrLike, `can not find file entity "file" from datastore for config set: services/service, revision: latest`)
			})
		})
	})

	Convey("File.Load", t, func() {
		ctx := memory.UseWithAppID(context.Background(), "dev~app-id")
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockGsClient := clients.NewMockGsClient(ctl)
		ctx = clients.WithGsClient(ctx, mockGsClient)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		Convey("by path and revision", func() {
			content, err := gzipCompress([]byte("content"))
			So(err, ShouldBeNil)
			So(datastore.Put(ctx, &File{
				Path:     "file",
				Revision: datastore.MakeKey(ctx, ConfigSetKind, "services/service", RevisionKind, "rev"),
				Content:  content,
			}), ShouldBeNil)

			file := &File{
				Path:     "file",
				Revision: datastore.MakeKey(ctx, ConfigSetKind, "services/service", RevisionKind, "rev"),
			}
			So(file.Load(ctx, false), ShouldBeNil)
			So(file, ShouldResemble, &File{
				Path:     "file",
				Revision: datastore.MakeKey(ctx, ConfigSetKind, "services/service", RevisionKind, "rev"),
				Content:  []byte("content"),
			})
		})

		Convey("by content hash", func() {
			content, err := gzipCompress([]byte("content"))
			So(err, ShouldBeNil)
			So(datastore.Put(ctx, &File{
				Path:        "file",
				Revision:    datastore.MakeKey(ctx, ConfigSetKind, "services/service", RevisionKind, "rev"),
				ContentHash: "hash",
				Content:     content,
			}), ShouldBeNil)

			file := &File{
				ContentHash: "hash",
			}
			So(file.Load(ctx, false), ShouldBeNil)
			So(file, ShouldResemble, &File{
				Path:        "file",
				Revision:    datastore.MakeKey(ctx, ConfigSetKind, "services/service", RevisionKind, "rev"),
				ContentHash: "hash",
				Content:     []byte("content"),
			})
		})

		Convey("resolve GcsURI", func() {
			content, err := gzipCompress([]byte("content"))
			So(err, ShouldBeNil)
			So(datastore.Put(ctx, &File{
				Path:        "file",
				Revision:    datastore.MakeKey(ctx, ConfigSetKind, "services/service", RevisionKind, "rev"),
				ContentHash: "hash",
				GcsURI:      gs.MakePath("bucket", "object"),
			}), ShouldBeNil)

			file := &File{
				ContentHash: "hash",
			}

			mockGsClient.EXPECT().Read(gomock.Any(), gomock.Eq("bucket"), gomock.Eq("object")).Return(content, nil)

			So(file.Load(ctx, true), ShouldBeNil)
			So(file.Content, ShouldResemble, []byte("content"))
		})

		Convey("GCS error", func() {
			So(datastore.Put(ctx, &File{
				Path:        "file",
				Revision:    datastore.MakeKey(ctx, ConfigSetKind, "services/service", RevisionKind, "rev"),
				ContentHash: "hash",
				GcsURI:      gs.MakePath("bucket", "object"),
			}), ShouldBeNil)

			file := &File{
				ContentHash: "hash",
			}

			mockGsClient.EXPECT().Read(gomock.Any(), gomock.Eq("bucket"), gomock.Eq("object")).Return(nil, errors.New("GCS internal error"))

			So(file.Load(ctx, true), ShouldErrLike, "cannot read from gs://bucket/object: GCS internal error")
		})

		Convey("nil content", func() {
			So(datastore.Put(ctx, &File{
				Path:     "file",
				Revision: datastore.MakeKey(ctx, ConfigSetKind, "services/service", RevisionKind, "rev"),
			}), ShouldBeNil)

			file := &File{
				Path:     "file",
				Revision: datastore.MakeKey(ctx, ConfigSetKind, "services/service", RevisionKind, "rev"),
			}
			So(file.Load(ctx, false), ShouldErrLike, "file content is nil. Might be damaged?")
		})

		Convey("empty content", func() {
			content, err := gzipCompress([]byte(""))
			So(err, ShouldBeNil)
			So(datastore.Put(ctx, &File{
				Path:        "file",
				Revision:    datastore.MakeKey(ctx, ConfigSetKind, "services/service", RevisionKind, "rev"),
				ContentHash: "hash",
				Content:     content,
			}), ShouldBeNil)

			file := &File{
				ContentHash: "hash",
			}
			So(file.Load(ctx, false), ShouldBeNil)
			So(file, ShouldResemble, &File{
				Path:        "file",
				Revision:    datastore.MakeKey(ctx, ConfigSetKind, "services/service", RevisionKind, "rev"),
				ContentHash: "hash",
				Content:     []byte(""),
			})
		})

		Convey("empty bytes", func() {
			content, err := gzipCompress([]byte{})
			So(err, ShouldBeNil)
			So(datastore.Put(ctx, &File{
				Path:        "file",
				Revision:    datastore.MakeKey(ctx, ConfigSetKind, "services/service", RevisionKind, "rev"),
				ContentHash: "hash",
				Content:     content,
			}), ShouldBeNil)

			file := &File{
				ContentHash: "hash",
			}
			So(file.Load(ctx, false), ShouldBeNil)
			So(file, ShouldResemble, &File{
				Path:        "file",
				Revision:    datastore.MakeKey(ctx, ConfigSetKind, "services/service", RevisionKind, "rev"),
				ContentHash: "hash",
				Content:     []byte{},
			})
		})

		Convey("miss required field", func() {
			file := &File{}
			So(file.Load(ctx, false), ShouldErrLike, "One of ContentHash or (path and revision) is required")
		})

		Convey("not found (path+revision)", func() {
			file := &File{
				Path:     "file",
				Revision: datastore.MakeKey(ctx, ConfigSetKind, "services/service", RevisionKind, "rev"),
			}
			So(file.Load(ctx, false), ShouldErrLike, `can not find file entity "file" from datastore for config set: services/service, revision: rev`)
		})

		Convey("not found (hash)", func() {
			file := &File{
				ContentHash: "hash",
			}
			So(file.Load(ctx, false), ShouldErrLike, `can not find matching file entity from datastore with hash "hash"`)
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
