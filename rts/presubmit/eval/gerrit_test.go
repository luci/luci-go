// Copyright 2020 The LUCI Authors.
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

package eval

import (
	"testing"

	"golang.org/x/net/context"
	"golang.org/x/time/rate"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestURLs(t *testing.T) {
	t.Parallel()
	Convey(`URLs`, t, func() {
		patchSet := GerritPatchset{
			Change: GerritChange{
				Host:   "example.googlesource.com",
				Number: 123,
			},
			Patchset: 4,
		}
		So(patchSet.Change.String(), ShouldEqual, "https://example.googlesource.com/c/123")
		So(patchSet.String(), ShouldEqual, "https://example.googlesource.com/c/123/4")
	})
}

func TestGerritClient(t *testing.T) {
	t.Parallel()
	Convey(`GerritClient`, t, func() {
		ctx := context.Background()
		client := &gerritClient{
			limiter: rate.NewLimiter(100, 1),
		}
		gerritPS := &GerritPatchset{
			Change: GerritChange{
				Host:    "example.googlesource.com",
				Project: "repo",
				Number:  123,
			},
			Patchset: 1,
		}

		Convey(`Works`, func() {
			var actualHost string
			var actualReq *gerritpb.ListFilesRequest
			client.listFilesRPC = func(ctx context.Context, host string, req *gerritpb.ListFilesRequest) (*gerritpb.ListFilesResponse, error) {
				actualHost = host
				actualReq = req
				return &gerritpb.ListFilesResponse{
					Files: map[string]*gerritpb.FileInfo{
						"a.go": {},
						"b.go": {},
					},
				}, nil
			}

			files, err := client.ChangedFiles(ctx, gerritPS)
			So(err, ShouldBeNil)
			So(files, ShouldResemble, []string{"a.go", "b.go"})
			So(actualHost, ShouldEqual, "example.googlesource.com")
			So(actualReq, ShouldResembleProto, &gerritpb.ListFilesRequest{
				Project:    "repo",
				Number:     123,
				RevisionId: "1",
			})
		})

		Convey(`CL not found`, func() {
			client.listFilesRPC = func(ctx context.Context, host string, req *gerritpb.ListFilesRequest) (*gerritpb.ListFilesResponse, error) {
				return nil, status.Errorf(codes.NotFound, "not found")
			}

			_, err := client.ChangedFiles(ctx, gerritPS)
			So(err, ShouldNotBeNil)
			So(psNotFound.In(err), ShouldBeTrue)
		})

		Convey(`Quota errors`, func() {
			returnQuotaError := true
			client.listFilesRPC = func(ctx context.Context, host string, req *gerritpb.ListFilesRequest) (*gerritpb.ListFilesResponse, error) {
				if returnQuotaError {
					returnQuotaError = false
					return nil, status.Errorf(codes.ResourceExhausted, "quota exhausted")
				}

				return &gerritpb.ListFilesResponse{
					Files: map[string]*gerritpb.FileInfo{
						"a.go": {},
						"b.go": {},
					},
				}, nil
			}

			files, err := client.ChangedFiles(ctx, gerritPS)
			So(err, ShouldBeNil)
			So(files, ShouldResemble, []string{"a.go", "b.go"})
		})
	})
}
