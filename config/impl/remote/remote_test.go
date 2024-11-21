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
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcGzip "google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"

	"go.chromium.org/luci/config"
	pb "go.chromium.org/luci/config_service/proto"
)

func TestRemoteCalls(t *testing.T) {
	t.Parallel()

	ftt.Run("Remote calls", t, func(t *ftt.Test) {
		ctl := gomock.NewController(t)
		mockClient := pb.NewMockConfigsClient(ctl)
		impl := remoteImpl{
			grpcClient: mockClient,
			httpClient: http.DefaultClient,
		}
		ctx := context.Background()

		t.Run("GetConfig", func(t *ftt.Test) {
			t.Run("ok - raw content", func(t *ftt.Test) {
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

				cfg, err := impl.GetConfig(ctx, config.Set("projects/project1"), "config.cfg", false)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cfg, should.Resemble(&config.Config{
					Meta: config.Meta{
						ConfigSet:   "projects/project1",
						Path:        "config.cfg",
						ContentHash: "sha256",
						Revision:    "revision",
						ViewURL:     "url",
					},
					Content: "content",
				}))
			})

			t.Run("ok - signed url", func(t *ftt.Test) {
				signedURLServer := signedURLServer(t, "content")
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

				cfg, err := impl.GetConfig(ctx, config.Set("projects/project1"), "config.cfg", false)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cfg, should.Resemble(&config.Config{
					Meta: config.Meta{
						ConfigSet:   "projects/project1",
						Path:        "config.cfg",
						ContentHash: "sha256",
						Revision:    "revision",
						ViewURL:     "url",
					},
					Content: "content",
				}))
			})

			t.Run("ok - meta only", func(t *ftt.Test) {
				mockClient.EXPECT().GetConfig(gomock.Any(), proto.MatcherEqual(&pb.GetConfigRequest{
					ConfigSet: "projects/project1",
					Path:      "config.cfg",
					Fields: &fieldmaskpb.FieldMask{
						Paths: []string{"config_set", "path", "content_sha256", "revision", "url"},
					},
				}), grpc.UseCompressor(grpcGzip.Name)).Return(&pb.Config{
					ConfigSet:     "projects/project1",
					Path:          "config.cfg",
					Revision:      "revision",
					ContentSha256: "sha256",
					Url:           "url",
				}, nil)

				cfg, err := impl.GetConfig(ctx, config.Set("projects/project1"), "config.cfg", true)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cfg, should.Resemble(&config.Config{
					Meta: config.Meta{
						ConfigSet:   "projects/project1",
						Path:        "config.cfg",
						ContentHash: "sha256",
						Revision:    "revision",
						ViewURL:     "url",
					},
				}))
			})

			t.Run("error - not found", func(t *ftt.Test) {
				mockClient.EXPECT().GetConfig(gomock.Any(), gomock.Any(), grpc.UseCompressor(grpcGzip.Name)).Return(nil, status.Errorf(codes.NotFound, "not found"))

				cfg, err := impl.GetConfig(ctx, config.Set("projects/project1"), "config.cfg", true)

				assert.Loosely(t, cfg, should.BeNil)
				assert.Loosely(t, err, should.ErrLike(config.ErrNoConfig))
			})

			t.Run("error - other", func(t *ftt.Test) {
				mockClient.EXPECT().GetConfig(gomock.Any(), gomock.Any(), grpc.UseCompressor(grpcGzip.Name)).Return(nil, status.Errorf(codes.Internal, "internal error"))

				cfg, err := impl.GetConfig(ctx, config.Set("projects/project1"), "config.cfg", true)

				assert.Loosely(t, cfg, should.BeNil)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.Internal))
				assert.Loosely(t, err, should.ErrLike("internal error"))
				assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
			})
		})

		t.Run("GetProjectConfigs", func(t *ftt.Test) {

			t.Run("ok - meta only", func(t *ftt.Test) {
				mockClient.EXPECT().GetProjectConfigs(gomock.Any(), proto.MatcherEqual(&pb.GetProjectConfigsRequest{
					Path: "config.cfg",
					Fields: &fieldmaskpb.FieldMask{
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

				configs, err := impl.GetProjectConfigs(ctx, "config.cfg", true)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, configs, should.Resemble([]config.Config{
					{
						Meta: config.Meta{
							ConfigSet:   "projects/project1",
							Path:        "config.cfg",
							ContentHash: "sha256",
							Revision:    "revision",
							ViewURL:     "url",
						},
					},
				}))
			})

			t.Run("ok - raw + signed url", func(t *ftt.Test) {
				signedURLServer := signedURLServer(t, "large content")
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

				configs, err := impl.GetProjectConfigs(ctx, "config.cfg", false)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, configs, should.Resemble([]config.Config{
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
				}))
			})

			t.Run("empty response", func(t *ftt.Test) {
				mockClient.EXPECT().GetProjectConfigs(gomock.Any(), proto.MatcherEqual(&pb.GetProjectConfigsRequest{
					Path: "config.cfg",
				}), grpc.UseCompressor(grpcGzip.Name)).Return(&pb.GetProjectConfigsResponse{}, nil)

				configs, err := impl.GetProjectConfigs(ctx, "config.cfg", false)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, configs, should.BeEmpty)
			})

			t.Run("rpc error", func(t *ftt.Test) {
				mockClient.EXPECT().GetProjectConfigs(gomock.Any(), proto.MatcherEqual(&pb.GetProjectConfigsRequest{
					Path: "config.cfg",
				}), grpc.UseCompressor(grpcGzip.Name)).Return(nil, status.Errorf(codes.Internal, "config server internal error"))

				configs, err := impl.GetProjectConfigs(ctx, "config.cfg", false)
				assert.Loosely(t, configs, should.BeNil)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.Internal))
				assert.Loosely(t, err, should.ErrLike("config server internal error"))
				assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
			})

			t.Run("signed url error", func(t *ftt.Test) {
				signedURLSever := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if strings.HasSuffix(r.URL.String(), "err") {
						w.WriteHeader(http.StatusInternalServerError)
						_, err := w.Write([]byte("internal error"))
						assert.Loosely(t, err, should.BeNil)
						return
					}
					buf := &bytes.Buffer{}
					gw := gzip.NewWriter(buf)
					_, err := gw.Write([]byte("large content"))
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, gw.Close(), should.BeNil)
					w.Header().Set("Content-Encoding", "gzip")
					_, err = w.Write(buf.Bytes())
					assert.Loosely(t, err, should.BeNil)
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

				configs, err := impl.GetProjectConfigs(ctx, "config.cfg", false)
				assert.Loosely(t, configs, should.BeNil)
				assert.Loosely(t, err, should.ErrLike(`for file(config.cfg) in config_set(projects/project2): failed to download file, got http response code: 500, body: "internal error"`))
				assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
			})
		})

		t.Run("GetProjects", func(t *ftt.Test) {
			t.Run("ok", func(t *ftt.Test) {
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

				projects, err := impl.GetProjects(ctx)
				assert.Loosely(t, err, should.BeNil)

				url1, err := url.Parse(res.ConfigSets[0].Url)
				assert.Loosely(t, err, should.BeNil)
				url2, err := url.Parse(res.ConfigSets[1].Url)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, projects, should.Resemble([]config.Project{
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
				}))
			})

			t.Run("rpc err", func(t *ftt.Test) {
				mockClient.EXPECT().ListConfigSets(gomock.Any(), proto.MatcherEqual(&pb.ListConfigSetsRequest{
					Domain: pb.ListConfigSetsRequest_PROJECT,
				})).Return(nil, status.Errorf(codes.Internal, "server internal error"))

				projects, err := impl.GetProjects(ctx)
				assert.Loosely(t, projects, should.BeNil)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.Internal))
				assert.Loosely(t, err, should.ErrLike("server internal error"))
				assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
			})
		})

		t.Run("ListFiles", func(t *ftt.Test) {
			t.Run("ok", func(t *ftt.Test) {
				mockClient.EXPECT().GetConfigSet(gomock.Any(), proto.MatcherEqual(&pb.GetConfigSetRequest{
					ConfigSet: "projects/project",
					Fields: &fieldmaskpb.FieldMask{
						Paths: []string{"configs"},
					},
				})).Return(&pb.ConfigSet{
					Configs: []*pb.Config{
						{Path: "file1"},
						{Path: "file2"},
					},
				}, nil)

				files, err := impl.ListFiles(ctx, config.Set("projects/project"))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, files, should.Resemble([]string{"file1", "file2"}))
			})

			t.Run("rpc err", func(t *ftt.Test) {
				mockClient.EXPECT().GetConfigSet(gomock.Any(), proto.MatcherEqual(&pb.GetConfigSetRequest{
					ConfigSet: "projects/project",
					Fields: &fieldmaskpb.FieldMask{
						Paths: []string{"configs"},
					},
				})).Return(nil, status.Errorf(codes.Internal, "server internal error"))

				files, err := impl.ListFiles(ctx, config.Set("projects/project"))
				assert.Loosely(t, files, should.BeNil)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.Internal))
				assert.Loosely(t, err, should.ErrLike("server internal error"))
				assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
			})
		})

		t.Run("GetConfigs", func(t *ftt.Test) {
			t.Run("listing err", func(t *ftt.Test) {
				mockClient.EXPECT().GetConfigSet(gomock.Any(), proto.MatcherEqual(&pb.GetConfigSetRequest{
					ConfigSet: "projects/project",
					Fields: &fieldmaskpb.FieldMask{
						Paths: []string{"configs"},
					},
				})).Return(nil, status.Errorf(codes.NotFound, "no config set"))
				files, err := impl.GetConfigs(ctx, "projects/project", nil, false)
				assert.Loosely(t, files, should.BeNil)
				assert.Loosely(t, err, should.Equal(config.ErrNoConfig))
			})

			t.Run("listing ok", func(t *ftt.Test) {
				expectCall := func() {
					mockClient.EXPECT().GetConfigSet(gomock.Any(), proto.MatcherEqual(&pb.GetConfigSetRequest{
						ConfigSet: "projects/project",
						Fields: &fieldmaskpb.FieldMask{
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

				}
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

				t.Run("meta only", func(t *ftt.Test) {
					expectCall()
					files, err := impl.GetConfigs(ctx, "projects/project", filter, true)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, files, should.Resemble(expectedOutput(true)))
				})

				t.Run("small bodies", func(t *ftt.Test) {
					expectCall()
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

					files, err := impl.GetConfigs(ctx, "projects/project", filter, false)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, files, should.Resemble(expectedOutput(false)))
				})

				t.Run("single fetch err", func(t *ftt.Test) {
					expectCall()
					expectGetConfigCall("file1-hash", nil, &pb.Config{
						Content: &pb.Config_RawContent{
							RawContent: []byte("file1 content"),
						},
					})
					expectGetConfigCall("file2-hash",
						status.Errorf(codes.Internal, "server internal error"), nil)

					files, err := impl.GetConfigs(ctx, "projects/project", filter, false)
					assert.Loosely(t, files, should.BeNil)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.Internal))
					assert.Loosely(t, err, should.ErrLike("server internal error"))
					assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
				})

				t.Run("single fetch unexpectedly missing", func(t *ftt.Test) {
					expectCall()
					expectGetConfigCall("file1-hash", nil, &pb.Config{
						Content: &pb.Config_RawContent{
							RawContent: []byte("file1 content"),
						},
					})
					expectGetConfigCall("file2-hash",
						status.Errorf(codes.NotFound, "gone but why"), nil)

					files, err := impl.GetConfigs(ctx, "projects/project", filter, false)
					assert.Loosely(t, files, should.BeNil)
					assert.Loosely(t, err, should.ErrLike("is unexpectedly gone"))
					assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
				})

				t.Run("large body - ok", func(t *ftt.Test) {
					expectCall()
					signedURLServer := signedURLServer(t, "file2 content")
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

					files, err := impl.GetConfigs(ctx, "projects/project", filter, false)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, files, should.Resemble(expectedOutput(false)))
				})

				t.Run("large body - err", func(t *ftt.Test) {
					expectCall()
					signedURLServer := signedURLServer(t, "file2 content")
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

					files, err := impl.GetConfigs(ctx, "projects/project", filter, false)
					assert.Loosely(t, files, should.BeNil)
					assert.Loosely(t, err, should.ErrLike(`fetching "file2" from signed URL: failed to download file`))
					assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
				})
			})
		})
	})
}

func signedURLServer(t testing.TB, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.String(), "err") {
			w.WriteHeader(http.StatusInternalServerError)
			_, err := w.Write([]byte(body))
			assert.Loosely(t, err, should.BeNil)
			return
		}
		buf := &bytes.Buffer{}
		gw := gzip.NewWriter(buf)
		_, err := gw.Write([]byte(body))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, gw.Close(), should.BeNil)
		w.Header().Set("Content-Encoding", "gzip")
		_, err = w.Write(buf.Bytes())
		assert.Loosely(t, err, should.BeNil)
	}))
}
