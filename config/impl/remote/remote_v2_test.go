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
	"go.chromium.org/luci/common/retry/transient"
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
				signedURLServer := signedURLServer(c, "content")
				defer signedURLServer.Close()

				mockClient.EXPECT().GetConfig(gomock.Any(), proto.MatcherEqual(&pb.GetConfigRequest{
					ConfigSet: "projects/project1",
					Path:      "config.cfg",
				}), grpc.UseCompressor(grpcGzip.Name)).Return(&pb.Config{
					ConfigSet: "projects/project1",
					Path:      "config.cfg",
					Content: &pb.Config_SignedUrl{
						SignedUrl: signedURLServer.URL,
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
				So(transient.Tag.In(err), ShouldBeTrue)
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
				signedURLServer := signedURLServer(c, "large content")
				defer signedURLServer.Close()

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
								SignedUrl: signedURLServer.URL,
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
				So(transient.Tag.In(err), ShouldBeTrue)
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
				So(err, ShouldErrLike, `for file(config.cfg) in config_set(projects/project2): failed to download file, got http response code: 500, body: "internal error"`)
				So(transient.Tag.In(err), ShouldBeTrue)
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
				So(transient.Tag.In(err), ShouldBeTrue)
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
				So(transient.Tag.In(err), ShouldBeTrue)
			})
		})

		Convey("GetConfigs", func() {
			Convey("listing err", func() {
				mockClient.EXPECT().GetConfigSet(gomock.Any(), proto.MatcherEqual(&pb.GetConfigSetRequest{
					ConfigSet: "projects/project",
					Fields: &field_mask.FieldMask{
						Paths: []string{"configs"},
					},
				})).Return(nil, status.Errorf(codes.NotFound, "no config set"))
				files, err := v2Impl.GetConfigs(ctx, "projects/project", nil, false)
				So(files, ShouldBeNil)
				So(err, ShouldEqual, config.ErrNoConfig)
			})

			Convey("listing ok", func() {
				mockClient.EXPECT().GetConfigSet(gomock.Any(), proto.MatcherEqual(&pb.GetConfigSetRequest{
					ConfigSet: "projects/project",
					Fields: &field_mask.FieldMask{
						Paths: []string{"configs"},
					},
				})).Return(&pb.ConfigSet{
					Configs: []*pb.Config{
						{
							ConfigSet:     "projects/project",
							Path:          "file1",
							ContentSha256: "file1-hash",
							Size:          123,
							Revision:      "rev",
							Url:           "file1-url",
						},
						{
							ConfigSet: "projects/project",
							Path:      "ignored",
							Revision:  "rev",
						},
						{
							ConfigSet:     "projects/project",
							Path:          "file2",
							ContentSha256: "file2-hash",
							Size:          456,
							Revision:      "rev",
							Url:           "file2-url",
						},
					},
				}, nil)

				filter := func(path string) bool { return path != "ignored" }

				expectedOutput := func(metaOnly bool) map[string]config.Config {
					content := func(p string) string { return "" }
					if !metaOnly {
						content = func(p string) string { return p + " content" }
					}
					return map[string]config.Config{
						"file1": {
							Meta: config.Meta{
								ConfigSet:   "projects/project",
								Path:        "file1",
								ContentHash: "file1-hash",
								Revision:    "rev",
								ViewURL:     "file1-url",
							},
							Content: content("file1"),
						},
						"file2": {
							Meta: config.Meta{
								ConfigSet:   "projects/project",
								Path:        "file2",
								ContentHash: "file2-hash",
								Revision:    "rev",
								ViewURL:     "file2-url",
							},
							Content: content("file2"),
						},
					}
				}

				expectGetConfigCall := func(hash string, err error, cfg *pb.Config) {
					if cfg != nil {
						cfg.ConfigSet = "ignore-me"
						cfg.Path = "ignore-me"
						cfg.Revision = "ignore-me"
						cfg.ContentSha256 = "ignore-me"
						cfg.Url = "ignore-me"
					}
					mockClient.EXPECT().GetConfig(gomock.Any(), proto.MatcherEqual(&pb.GetConfigRequest{
						ConfigSet:     "projects/project",
						ContentSha256: hash,
					}), grpc.UseCompressor(grpcGzip.Name)).Return(cfg, err)
				}

				Convey("meta only", func() {
					files, err := v2Impl.GetConfigs(ctx, "projects/project", filter, true)
					So(err, ShouldBeNil)
					So(files, ShouldResemble, expectedOutput(true))
				})

				Convey("small bodies", func() {
					expectGetConfigCall("file1-hash", nil, &pb.Config{
						Content: &pb.Config_RawContent{
							RawContent: []byte("file1 content"),
						},
					})
					expectGetConfigCall("file2-hash", nil, &pb.Config{
						Content: &pb.Config_RawContent{
							RawContent: []byte("file2 content"),
						},
					})

					files, err := v2Impl.GetConfigs(ctx, "projects/project", filter, false)
					So(err, ShouldBeNil)
					So(files, ShouldResemble, expectedOutput(false))
				})

				Convey("single fetch err", func() {
					expectGetConfigCall("file1-hash", nil, &pb.Config{
						Content: &pb.Config_RawContent{
							RawContent: []byte("file1 content"),
						},
					})
					expectGetConfigCall("file2-hash",
						status.Errorf(codes.Internal, "server internal error"), nil)

					files, err := v2Impl.GetConfigs(ctx, "projects/project", filter, false)
					So(files, ShouldBeNil)
					So(err, ShouldHaveGRPCStatus, codes.Internal, "server internal error")
					So(transient.Tag.In(err), ShouldBeTrue)
				})

				Convey("single fetch unexpectedly missing", func() {
					expectGetConfigCall("file1-hash", nil, &pb.Config{
						Content: &pb.Config_RawContent{
							RawContent: []byte("file1 content"),
						},
					})
					expectGetConfigCall("file2-hash",
						status.Errorf(codes.NotFound, "gone but why"), nil)

					files, err := v2Impl.GetConfigs(ctx, "projects/project", filter, false)
					So(files, ShouldBeNil)
					So(err, ShouldErrLike, "is unexpectedly gone")
					So(transient.Tag.In(err), ShouldBeFalse)
				})

				Convey("large body - ok", func(c C) {
					signedURLServer := signedURLServer(c, "file2 content")
					defer signedURLServer.Close()

					expectGetConfigCall("file1-hash", nil, &pb.Config{
						Content: &pb.Config_RawContent{
							RawContent: []byte("file1 content"),
						},
					})
					expectGetConfigCall("file2-hash", nil, &pb.Config{
						Content: &pb.Config_SignedUrl{
							SignedUrl: signedURLServer.URL,
						},
					})

					files, err := v2Impl.GetConfigs(ctx, "projects/project", filter, false)
					So(err, ShouldBeNil)
					So(files, ShouldResemble, expectedOutput(false))
				})

				Convey("large body - err", func(c C) {
					signedURLServer := signedURLServer(c, "file2 content")
					defer signedURLServer.Close()

					expectGetConfigCall("file1-hash", nil, &pb.Config{
						Content: &pb.Config_RawContent{
							RawContent: []byte("file1 content"),
						},
					})
					expectGetConfigCall("file2-hash", nil, &pb.Config{
						Content: &pb.Config_SignedUrl{
							SignedUrl: signedURLServer.URL + "/err",
						},
					})

					files, err := v2Impl.GetConfigs(ctx, "projects/project", filter, false)
					So(files, ShouldBeNil)
					So(err, ShouldErrLike, `fetching "file2" from signed URL: failed to download file`)
					So(transient.Tag.In(err), ShouldBeTrue)
				})
			})
		})
	})
}

func signedURLServer(c C, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.String(), "err") {
			w.WriteHeader(http.StatusInternalServerError)
			_, err := w.Write([]byte(body))
			c.So(err, ShouldBeNil)
			return
		}
		buf := &bytes.Buffer{}
		gw := gzip.NewWriter(buf)
		_, err := gw.Write([]byte(body))
		c.So(err, ShouldBeNil)
		c.So(gw.Close(), ShouldBeNil)
		w.Header().Set("Content-Encoding", "gzip")
		_, err = w.Write(buf.Bytes())
		c.So(err, ShouldBeNil)
	}))
}
