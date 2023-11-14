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

package remote

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/klauspost/compress/gzip"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcGzip "google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/proto"
	pb "go.chromium.org/luci/config_service/proto"

	"go.chromium.org/luci/config"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRemoteV2Calls(t *testing.T) {
	t.Parallel()

	Convey("Remote V2 calls", t, func() {
		ctl := gomock.NewController(t)
		mockClient := pb.NewMockConfigsClient(ctl)
		v2Impl := remoteV2Impl{
			grpcClient: mockClient,
			httpClient: http.DefaultClient,
		}
		ctx := context.Background()

		Convey("GetConfig", func() {
			Convey("ok - raw content", func() {
				mockClient.EXPECT().GetConfig(gomock.Any(), proto.MatcherEqual(&pb.GetConfigRequest{
					ConfigSet: "projects/project1",
					Path:      "config.cfg",
				}), grpc.UseCompressor(grpcGzip.Name)).Return(&pb.Config{
					ConfigSet: "projects/project1",
					Path:      "config.cfg",
					Content: &pb.Config_RawContent{
						RawContent: []byte("content"),
					},
					Revision:      "revision",
					ContentSha256: "sha256",
					Url:           "url",
				}, nil)

				cfg, err := v2Impl.GetConfig(ctx, config.Set("projects/project1"), "config.cfg", false)

				So(err, ShouldBeNil)
				So(cfg, ShouldResemble, &config.Config{
					Meta: config.Meta{
						ConfigSet:   "projects/project1",
						Path:        "config.cfg",
						ContentHash: "sha256",
						Revision:    "revision",
						ViewURL:     "url",
					},
					Content: "content",
				})
			})

			Convey("ok - signed url", func(c C) {
				signedURLSever := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					buf := &bytes.Buffer{}
					gw := gzip.NewWriter(buf)
					_, err := gw.Write([]byte("content"))
					c.So(err, ShouldBeNil)
					c.So(gw.Close(), ShouldBeNil)
					w.Header().Set("Content-Encoding", "gzip")
					_, err = w.Write(buf.Bytes())
					c.So(err, ShouldBeNil)
				}))
				defer signedURLSever.Close()

				mockClient.EXPECT().GetConfig(gomock.Any(), proto.MatcherEqual(&pb.GetConfigRequest{
					ConfigSet: "projects/project1",
					Path:      "config.cfg",
				}), grpc.UseCompressor(grpcGzip.Name)).Return(&pb.Config{
					ConfigSet: "projects/project1",
					Path:      "config.cfg",
					Content: &pb.Config_SignedUrl{
						SignedUrl: signedURLSever.URL,
					},
					Revision:      "revision",
					ContentSha256: "sha256",
					Url:           "url",
				}, nil)

				cfg, err := v2Impl.GetConfig(ctx, config.Set("projects/project1"), "config.cfg", false)

				So(err, ShouldBeNil)
				So(cfg, ShouldResemble, &config.Config{
					Meta: config.Meta{
						ConfigSet:   "projects/project1",
						Path:        "config.cfg",
						ContentHash: "sha256",
						Revision:    "revision",
						ViewURL:     "url",
					},
					Content: "content",
				})
			})

			Convey("ok - meta only", func() {
				mockClient.EXPECT().GetConfig(gomock.Any(), proto.MatcherEqual(&pb.GetConfigRequest{
					ConfigSet: "projects/project1",
					Path:      "config.cfg",
					Fields: &field_mask.FieldMask{
						Paths: []string{"config_set", "path", "content_sha256", "revision", "url"},
					},
				}), grpc.UseCompressor(grpcGzip.Name)).Return(&pb.Config{
					ConfigSet:     "projects/project1",
					Path:          "config.cfg",
					Revision:      "revision",
					ContentSha256: "sha256",
					Url:           "url",
				}, nil)

				cfg, err := v2Impl.GetConfig(ctx, config.Set("projects/project1"), "config.cfg", true)

				So(err, ShouldBeNil)
				So(cfg, ShouldResemble, &config.Config{
					Meta: config.Meta{
						ConfigSet:   "projects/project1",
						Path:        "config.cfg",
						ContentHash: "sha256",
						Revision:    "revision",
						ViewURL:     "url",
					},
				})
			})

			Convey("error - not found", func() {
				mockClient.EXPECT().GetConfig(gomock.Any(), gomock.Any(), grpc.UseCompressor(grpcGzip.Name)).Return(nil, status.Errorf(codes.NotFound, "not found"))

				cfg, err := v2Impl.GetConfig(ctx, config.Set("projects/project1"), "config.cfg", true)

				So(cfg, ShouldBeNil)
				So(err, ShouldErrLike, config.ErrNoConfig)
			})

			Convey("error - other", func() {
				mockClient.EXPECT().GetConfig(gomock.Any(), gomock.Any(), grpc.UseCompressor(grpcGzip.Name)).Return(nil, status.Errorf(codes.Internal, "internal error"))

				cfg, err := v2Impl.GetConfig(ctx, config.Set("projects/project1"), "config.cfg", true)

				So(cfg, ShouldBeNil)
				So(err, ShouldHaveGRPCStatus, codes.Internal, "internal error")
			})
		})

		Convey("GetProjectConfigs", func() {

			Convey("ok - meta only", func() {
				mockClient.EXPECT().GetProjectConfigs(gomock.Any(), proto.MatcherEqual(&pb.GetProjectConfigsRequest{
					Path: "config.cfg",
					Fields: &field_mask.FieldMask{
						Paths: []string{"config_set", "path", "content_sha256", "revision", "url"},
					},
				}), grpc.UseCompressor(grpcGzip.Name)).Return(&pb.GetProjectConfigsResponse{
					Configs: []*pb.Config{
						{
							ConfigSet:     "projects/project1",
							Path:          "config.cfg",
							Revision:      "revision",
							ContentSha256: "sha256",
							Url:           "url",
						},
					},
				}, nil)

				configs, err := v2Impl.GetProjectConfigs(ctx, "config.cfg", true)
				So(err, ShouldBeNil)
				So(configs, ShouldResemble, []config.Config{
					{
						Meta: config.Meta{
							ConfigSet:   "projects/project1",
							Path:        "config.cfg",
							ContentHash: "sha256",
							Revision:    "revision",
							ViewURL:     "url",
						},
					},
				})
			})

			Convey("ok - raw + signed url", func(c C) {
				signedURLSever := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					buf := &bytes.Buffer{}
					gw := gzip.NewWriter(buf)
					_, err := gw.Write([]byte("large content"))
					c.So(err, ShouldBeNil)
					c.So(gw.Close(), ShouldBeNil)
					w.Header().Set("Content-Encoding", "gzip")
					_, err = w.Write(buf.Bytes())
					c.So(err, ShouldBeNil)
				}))
				defer signedURLSever.Close()

				mockClient.EXPECT().GetProjectConfigs(gomock.Any(), proto.MatcherEqual(&pb.GetProjectConfigsRequest{
					Path: "config.cfg",
				}), grpc.UseCompressor(grpcGzip.Name)).Return(&pb.GetProjectConfigsResponse{
					Configs: []*pb.Config{
						{
							ConfigSet:     "projects/project1",
							Path:          "config.cfg",
							Revision:      "revision",
							ContentSha256: "sha256",
							Url:           "url",
							Content: &pb.Config_RawContent{
								RawContent: []byte("small content"),
							},
						},
						{
							ConfigSet:     "projects/project2",
							Path:          "config.cfg",
							Revision:      "revision",
							ContentSha256: "sha256",
							Url:           "url",
							Content: &pb.Config_SignedUrl{
								SignedUrl: signedURLSever.URL,
							},
						},
					},
				}, nil)

				configs, err := v2Impl.GetProjectConfigs(ctx, "config.cfg", false)
				So(err, ShouldBeNil)
				So(configs, ShouldResemble, []config.Config{
					{
						Meta: config.Meta{
							ConfigSet:   "projects/project1",
							Path:        "config.cfg",
							ContentHash: "sha256",
							Revision:    "revision",
							ViewURL:     "url",
						},
						Content: "small content",
					},
					{
						Meta: config.Meta{
							ConfigSet:   "projects/project2",
							Path:        "config.cfg",
							ContentHash: "sha256",
							Revision:    "revision",
							ViewURL:     "url",
						},
						Content: "large content",
					},
				})
			})

			Convey("empty response", func() {
				mockClient.EXPECT().GetProjectConfigs(gomock.Any(), proto.MatcherEqual(&pb.GetProjectConfigsRequest{
					Path: "config.cfg",
				}), grpc.UseCompressor(grpcGzip.Name)).Return(&pb.GetProjectConfigsResponse{}, nil)

				configs, err := v2Impl.GetProjectConfigs(ctx, "config.cfg", false)
				So(err, ShouldBeNil)
				So(configs, ShouldBeEmpty)
			})

			Convey("rpc error", func() {
				mockClient.EXPECT().GetProjectConfigs(gomock.Any(), proto.MatcherEqual(&pb.GetProjectConfigsRequest{
					Path: "config.cfg",
				}), grpc.UseCompressor(grpcGzip.Name)).Return(nil, status.Errorf(codes.Internal, "config server internal error"))

				configs, err := v2Impl.GetProjectConfigs(ctx, "config.cfg", false)
				So(configs, ShouldBeNil)
				So(err, ShouldHaveGRPCStatus, codes.Internal, "config server internal error")
			})

			Convey("signed url error", func(c C) {
				signedURLSever := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if strings.HasSuffix(r.URL.String(), "err") {
						w.WriteHeader(http.StatusInternalServerError)
						_, err := w.Write([]byte("internal error"))
						c.So(err, ShouldBeNil)
						return
					}
					buf := &bytes.Buffer{}
					gw := gzip.NewWriter(buf)
					_, err := gw.Write([]byte("large content"))
					c.So(err, ShouldBeNil)
					c.So(gw.Close(), ShouldBeNil)
					w.Header().Set("Content-Encoding", "gzip")
					_, err = w.Write(buf.Bytes())
					c.So(err, ShouldBeNil)
				}))
				defer signedURLSever.Close()

				mockClient.EXPECT().GetProjectConfigs(gomock.Any(), proto.MatcherEqual(&pb.GetProjectConfigsRequest{
					Path: "config.cfg",
				}), grpc.UseCompressor(grpcGzip.Name)).Return(&pb.GetProjectConfigsResponse{
					Configs: []*pb.Config{
						{
							ConfigSet: "projects/project1",
							Path:      "config.cfg",
							Content: &pb.Config_SignedUrl{
								SignedUrl: signedURLSever.URL,
							},
						},
						{
							ConfigSet: "projects/project2",
							Path:      "config.cfg",
							Content: &pb.Config_SignedUrl{
								SignedUrl: signedURLSever.URL + "/err",
							},
						},
					},
				}, nil)

				configs, err := v2Impl.GetProjectConfigs(ctx, "config.cfg", false)
				So(configs, ShouldBeNil)
				So(err, ShouldErrLike, `For file(config.cfg) in config_set(projects/project2): failed to download file, got http response code: 500, body: "internal error"`)
			})
		})

		Convey("GetProjects", func() {
			Convey("ok", func() {
				res := &pb.ListConfigSetsResponse{
					ConfigSets: []*pb.ConfigSet{
						{
							Name: "projects/project1",
							Url:  "https://a.googlesource.com/project1",
						},
						{
							Name: "projects/project2",
							Url:  "https://b.googlesource.com/project2",
						},
					},
				}
				mockClient.EXPECT().ListConfigSets(gomock.Any(), proto.MatcherEqual(&pb.ListConfigSetsRequest{
					Domain: pb.ListConfigSetsRequest_PROJECT,
				})).Return(res, nil)

				projects, err := v2Impl.GetProjects(ctx)
				So(err, ShouldBeNil)

				url1, err := url.Parse(res.ConfigSets[0].Url)
				So(err, ShouldBeNil)
				url2, err := url.Parse(res.ConfigSets[1].Url)
				So(err, ShouldBeNil)
				So(projects, ShouldResemble, []config.Project{
					{
						ID:       "project1",
						Name:     "project1",
						RepoType: config.GitilesRepo,
						RepoURL:  url1,
					},
					{
						ID:       "project2",
						Name:     "project2",
						RepoType: config.GitilesRepo,
						RepoURL:  url2,
					},
				})
			})

			Convey("rpc err", func() {
				mockClient.EXPECT().ListConfigSets(gomock.Any(), proto.MatcherEqual(&pb.ListConfigSetsRequest{
					Domain: pb.ListConfigSetsRequest_PROJECT,
				})).Return(nil, status.Errorf(codes.Internal, "server internal error"))

				projects, err := v2Impl.GetProjects(ctx)
				So(projects, ShouldBeNil)
				So(err, ShouldHaveGRPCStatus, codes.Internal, "server internal error")
			})
		})

		Convey("ListFiles", func() {
			Convey("ok", func() {
				mockClient.EXPECT().GetConfigSet(gomock.Any(), proto.MatcherEqual(&pb.GetConfigSetRequest{
					ConfigSet: "projects/project",
					Fields: &field_mask.FieldMask{
						Paths: []string{"configs"},
					},
				})).Return(&pb.ConfigSet{
					Configs: []*pb.Config{
						{Path: "file1"},
						{Path: "file2"},
					},
				}, nil)

				files, err := v2Impl.ListFiles(ctx, config.Set("projects/project"))
				So(err, ShouldBeNil)
				So(files, ShouldResemble, []string{"file1", "file2"})
			})

			Convey("rpc err", func() {
				mockClient.EXPECT().GetConfigSet(gomock.Any(), proto.MatcherEqual(&pb.GetConfigSetRequest{
					ConfigSet: "projects/project",
					Fields: &field_mask.FieldMask{
						Paths: []string{"configs"},
					},
				})).Return(nil, status.Errorf(codes.Internal, "server internal error"))

				files, err := v2Impl.ListFiles(ctx, config.Set("projects/project"))
				So(files, ShouldBeNil)
				So(err, ShouldHaveGRPCStatus, codes.Internal, "server internal error")
			})
		})
	})
}
