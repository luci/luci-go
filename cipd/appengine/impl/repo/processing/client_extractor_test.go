// Copyright 2018 The LUCI Authors.
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
	"io"
	"strings"
	"testing"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/gaetesting"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/gs"
	"go.chromium.org/luci/cipd/appengine/impl/model"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func packageReader(data map[string]string) *PackageReader {
	buf := bytes.NewReader(testutil.MakeZip(data))
	size := int64(buf.Len())
	r, _ := NewPackageReader(buf, size)
	return r
}

func instance(ctx context.Context, pkg string) *model.Instance {
	return &model.Instance{
		InstanceID: strings.Repeat("a", 40),
		Package:    model.PackageKey(ctx, pkg),
	}
}

func TestGetClientPackage(t *testing.T) {
	t.Parallel()

	Convey("works", t, func() {
		pkg, err := GetClientPackage("linux-amd64")
		So(err, ShouldBeNil)
		So(pkg, ShouldEqual, "infra/tools/cipd/linux-amd64")
	})

	Convey("fails", t, func() {
		_, err := GetClientPackage("../sneaky")
		So(err, ShouldErrLike, "invalid package name")
	})
}

func TestClientExtractor(t *testing.T) {
	t.Parallel()

	ctx := gaetesting.TestingContext()

	originalBody := strings.Repeat("01234567", 8960)
	expectedDigest := "390eb4008bd5261b63c5bf45c6750c0ad82efc17"
	goodPkg := packageReader(map[string]string{"cipd": originalBody})
	inst := instance(ctx, "infra/tools/cipd/linux-amd64")

	Convey("With mocks", t, func() {
		var publishedRef *api.ObjectRef
		var canceled bool

		cas := testutil.MockCAS{
			BeginUploadImpl: func(_ context.Context, r *api.BeginUploadRequest) (*api.UploadOperation, error) {
				So(r.HashAlgo, ShouldEqual, api.HashAlgo_SHA1)
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
			CancelUploadImpl: func(_ context.Context, r *api.CancelUploadRequest) (*api.UploadOperation, error) {
				So(r.UploadOperationId, ShouldEqual, "op_id")
				canceled = true
				return &api.UploadOperation{Status: api.UploadStatus_CANCELED}, nil
			},
		}

		extracted := bytes.Buffer{}
		uploader := &trackingWriter{w: &extracted}

		ce := ClientExtractor{
			CAS: &cas,

			uploader:   func(ctx context.Context, size int64, uploadURL string) io.Writer { return uploader },
			bufferSize: 64 * 1024,
		}

		Convey("Happy path", func() {
			res, err := ce.Run(ctx, inst, goodPkg)
			So(err, ShouldBeNil)
			So(res.Err, ShouldBeNil)

			r := res.Result.(ClientExtractorResult)
			So(r.ClientBinary.Size, ShouldEqual, len(originalBody))
			So(r.ClientBinary.HashAlgo, ShouldEqual, "SHA1")
			So(r.ClientBinary.HashDigest, ShouldEqual, expectedDigest)

			So(extracted.String(), ShouldEqual, originalBody)
			So(publishedRef, ShouldResembleProto, &api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA1,
				HexDigest: expectedDigest,
			})

			// Was written by 64 Kb chunks, NOT 32 Kb (as used by zip.Reader).
			So(uploader.calls, ShouldResemble, []int{64 * 1024, 6 * 1024})
		})

		Convey("No such file in the package", func() {
			res, err := ce.Run(ctx, inst, packageReader(nil))
			So(err, ShouldBeNil)
			So(res.Err, ShouldErrLike, `failed to open the file for reading: no file "cipd" inside the package`)
		})

		Convey("Internal error when initiating the upload", func() {
			cas.BeginUploadImpl = func(_ context.Context, r *api.BeginUploadRequest) (*api.UploadOperation, error) {
				return nil, grpc.Errorf(codes.Internal, "boo")
			}
			_, err := ce.Run(ctx, inst, goodPkg)
			So(err, ShouldErrLike, `failed to open a CAS upload: rpc error: code = Internal desc = boo`)
		})

		Convey("Internal error when finalizing the upload", func() {
			cas.FinishUploadImpl = func(_ context.Context, r *api.FinishUploadRequest) (*api.UploadOperation, error) {
				return nil, grpc.Errorf(codes.Internal, "boo")
			}
			_, err := ce.Run(ctx, inst, goodPkg)
			So(err, ShouldErrLike, `failed to finalize the CAS upload: rpc error: code = Internal desc = boo`)
		})

		Convey("Asked to restart the upload", func() {
			uploader.err = &gs.RestartUploadError{Offset: 124}

			_, err := ce.Run(ctx, inst, goodPkg)
			So(err, ShouldErrLike, `asked to restart the upload from faraway offset: the upload should be restarted from offset 124`)
			So(canceled, ShouldBeTrue)
		})
	})

	Convey("Applicable works", t, func() {
		ce := ClientExtractor{}
		So(ce.Applicable(instance(ctx, "infra/tools/cipd/linux")), ShouldBeTrue)
		So(ce.Applicable(instance(ctx, "infra/tools/stuff/linux")), ShouldBeFalse)
	})
}

func TestGetResult(t *testing.T) {
	t.Parallel()

	Convey("With datastore", t, func() {
		ctx := gaetesting.TestingContext()

		write := func(res *ClientExtractorResult, err string) {
			r := model.ProcessingResult{
				ProcID: ClientExtractorProcID,
				Instance: datastore.KeyForObj(ctx, &model.Instance{
					InstanceID: strings.Repeat("a", 40),
					Package:    model.PackageKey(ctx, "a/b/c"),
				}),
			}
			if res != nil {
				r.Success = true
				So(r.WriteResult(&res), ShouldBeNil)
			} else {
				r.Error = err
			}
			So(datastore.Put(ctx, &r), ShouldBeNil)
		}

		Convey("Happy path", func() {
			res := ClientExtractorResult{}
			res.ClientBinary.HashAlgo = "SHA1"
			res.ClientBinary.HashDigest = strings.Repeat("b", 40)
			write(&res, "")

			out, err := GetClientExtractorResult(ctx, &api.Instance{
				Package: "a/b/c",
				Instance: &api.ObjectRef{
					HashAlgo:  api.HashAlgo_SHA1,
					HexDigest: strings.Repeat("a", 40),
				},
			})
			So(err, ShouldBeNil)
			So(out, ShouldResemble, &res)

			ref, err := out.ToObjectRef()
			So(err, ShouldBeNil)
			So(ref, ShouldResemble, &api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA1,
				HexDigest: strings.Repeat("b", 40),
			})
		})

		Convey("Failed processor", func() {
			write(nil, "boom")

			_, err := GetClientExtractorResult(ctx, &api.Instance{
				Package: "a/b/c",
				Instance: &api.ObjectRef{
					HashAlgo:  api.HashAlgo_SHA1,
					HexDigest: strings.Repeat("a", 40),
				},
			})
			So(err, ShouldErrLike, "boom")
		})

		Convey("No result", func() {
			_, err := GetClientExtractorResult(ctx, &api.Instance{
				Package: "a/b/c",
				Instance: &api.ObjectRef{
					HashAlgo:  api.HashAlgo_SHA1,
					HexDigest: strings.Repeat("a", 40),
				},
			})
			So(err, ShouldEqual, datastore.ErrNoSuchEntity)
		})
	})
}

type trackingWriter struct {
	w     io.Writer
	calls []int // sizes of each pushed chunk
	err   error // err to return from Write
}

func (w *trackingWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	w.calls = append(w.calls, len(p))
	return w.w.Write(p)
}
