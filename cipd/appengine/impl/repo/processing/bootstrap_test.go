// Copyright 2021 The LUCI Authors.
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

package processing

import (
	"bytes"
	"context"
	"io"
	"testing"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"
	"go.chromium.org/luci/gae/impl/memory"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestBootstrapPackageExtractor(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func() {
		var publishedRef *api.ObjectRef

		ctx := memory.Use(context.Background())

		testInstance := instance(ctx, "some/pkg", api.HashAlgo_SHA256)
		testBody := "12345678"
		testBodyDigest := hexDigest(api.HashAlgo_SHA256, testBody)

		cas := testutil.MockCAS{
			BeginUploadImpl: func(_ context.Context, r *api.BeginUploadRequest) (*api.UploadOperation, error) {
				So(r.HashAlgo, ShouldEqual, api.HashAlgo_SHA256)
				return &api.UploadOperation{
					OperationId: "op_id",
					UploadUrl:   "http://example.com/upload",
				}, nil
			},
			FinishUploadImpl: func(_ context.Context, r *api.FinishUploadRequest) (*api.UploadOperation, error) {
				So(r.UploadOperationId, ShouldEqual, "op_id")
				publishedRef = r.ForceHash
				return &api.UploadOperation{Status: api.UploadStatus_PUBLISHED}, nil
			},
		}

		extracted := bytes.Buffer{}

		ex := BootstrapPackageExtractor{
			CAS:      &cas,
			uploader: func(ctx context.Context, size int64, uploadURL string) io.Writer { return &extracted },
		}

		Convey("Happy path", func() {
			res, err := ex.Run(ctx, testInstance, packageReader(map[string]string{
				"some_binary.exe":       testBody,
				"some_binary.bat":       "zzz",
				".cipdpkg/ignored":      "zzz",
				".cipdpkg/also ignored": "zzz",
			}))
			So(err, ShouldBeNil)
			So(res.Err, ShouldBeNil)

			So(res.Result.(BootstrapExtractorResult), ShouldResemble, BootstrapExtractorResult{
				File:       "some_binary.exe",
				HashAlgo:   "SHA256",
				HashDigest: testBodyDigest,
				Size:       int64(len(testBody)),
			})

			So(extracted.String(), ShouldEqual, testBody)
			So(publishedRef, ShouldResembleProto, &api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA256,
				HexDigest: testBodyDigest,
			})
		})

		Convey("Many files", func() {
			res, err := ex.Run(ctx, testInstance, packageReader(map[string]string{
				"some_binary.exe":    testBody,
				"another_binary.exe": testBody,
			}))
			So(err, ShouldBeNil)
			So(res.Err, ShouldErrLike, "contains multiple files")
		})

		Convey("No files", func() {
			res, err := ex.Run(ctx, testInstance, packageReader(map[string]string{
				".cipdpkg/ignored":      "zzz",
				".cipdpkg/also ignored": "zzz",
			}))
			So(err, ShouldBeNil)
			So(res.Err, ShouldErrLike, "contains no files")
		})

		Convey("Not at root", func() {
			res, err := ex.Run(ctx, testInstance, packageReader(map[string]string{
				"dir/some_binary.exe": testBody,
			}))
			So(err, ShouldBeNil)
			So(res.Err, ShouldErrLike, "not at the package root")
		})
	})
}
