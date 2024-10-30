// Copyright 2017 The LUCI Authors.
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

package cfgmodule

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/klauspost/compress/gzip"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestConsumerServer(t *testing.T) {
	t.Parallel()

	ftt.Run("ConsumerServer", t, func(t *ftt.Test) {
		const configSA = "luci-config-service@luci-config.iam.gserviceaccount.com"
		authState := &authtest.FakeState{
			Identity: "user:" + configSA,
		}
		ctx := authtest.MockAuthConfig(context.Background())
		ctx = auth.WithState(ctx, authState)
		rules := validation.NewRuleSet()
		srv := ConsumerServer{
			Rules: rules,
			GetConfigServiceAccountFn: func(ctx context.Context) (string, error) {
				return configSA, nil
			},
		}

		t.Run("Check caller", func(t *ftt.Test) {
			t.Run("Allow LUCI Config service account", func(t *ftt.Test) {
				_, err := srv.GetMetadata(ctx, &emptypb.Empty{})
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("Allow Admin group", func(t *ftt.Test) {
				authState := &authtest.FakeState{
					Identity:       "user:someone@example.com",
					IdentityGroups: []string{adminGroup},
				}
				ctx = auth.WithState(ctx, authState)
				_, err := srv.GetMetadata(ctx, &emptypb.Empty{})
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("Disallow", func(t *ftt.Test) {
				check := func(t testing.TB) {
					t.Helper()
					ctx = auth.WithState(ctx, authState)
					_, err := srv.GetMetadata(ctx, &emptypb.Empty{})
					assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied), truth.LineContext())
				}

				t.Run("Non-admin users", func(t *ftt.Test) {
					authState = &authtest.FakeState{
						Identity: "user:someone@example.com",
					}
					check(t)
				})
				t.Run("Anonymous", func(t *ftt.Test) {
					authState = &authtest.FakeState{
						Identity: identity.AnonymousIdentity,
					}
					check(t)
				})
			})
		})

		t.Run("GetMetadata", func(t *ftt.Test) {
			rules.Add("configSet", "path", nil)
			res, err := srv.GetMetadata(ctx, &emptypb.Empty{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&config.ServiceMetadata{
				ConfigPatterns: []*config.ConfigPattern{
					{
						ConfigSet: "exact:configSet",
						Path:      "exact:path",
					},
				},
			}))
		})

		t.Run("ValidateConfig", func(t *ftt.Test) {
			const configSet = "project/xyz"
			addRule := func(path string) {
				rules.Add(configSet, path, func(ctx *validation.Context, configSet, path string, content []byte) error {
					if bytes.Contains(content, []byte("good")) {
						return nil
					}
					if bytes.Contains(content, []byte("error")) {
						ctx.Errorf("blocking error")
					}
					if bytes.Contains(content, []byte("warning")) {
						ctx.Warningf("diagnostic warning")
					}
					return nil
				})
			}
			resources := map[string]struct {
				data    []byte
				gzipped bool
			}{}

			addFileToRemote := func(path string, data []byte, compress bool) {
				if compress {
					var b bytes.Buffer
					gw := gzip.NewWriter(&b)
					_, err := gw.Write(data)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, gw.Close(), should.BeNil)
					data = b.Bytes()
				}
				resources[path] = struct {
					data    []byte
					gzipped bool
				}{
					data:    data,
					gzipped: compress,
				}
			}

			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path := strings.TrimLeft(r.RequestURI, "/")
				var resp []byte
				switch resource, ok := resources[path]; {
				case !ok:
					http.Error(w, fmt.Sprintf("Unknown resource %q", path), http.StatusNotFound)
				case r.Header.Get("Accept-Encoding") == "gzip" && resource.gzipped:
					w.Header().Add("Content-Encoding", "gzip")
					resp = resource.data
				case resource.gzipped:
					gr, err := gzip.NewReader(bytes.NewBuffer(resource.data))
					if err != nil {
						http.Error(w, fmt.Sprintf("failed to create reader %s", err), http.StatusInternalServerError)
						return
					}
					defer func() { _ = gr.Close() }()
					resp, err = io.ReadAll(gr)
					if err != nil {
						http.Error(w, fmt.Sprintf("failed to read data %s", err), http.StatusInternalServerError)
						return
					}
				default:
					resp = resource.data
				}
				_, err := w.Write(resp)
				if err != nil {
					panic(err)
				}
			}))
			defer ts.Close()

			t.Run("Single file", func(t *ftt.Test) {
				const path = "some_file.cfg"
				addRule(path)
				file := &config.ValidateConfigsRequest_File{
					Path: path,
				}
				req := &config.ValidateConfigsRequest{
					ConfigSet: configSet,
					Files: &config.ValidateConfigsRequest_Files{
						Files: []*config.ValidateConfigsRequest_File{
							file,
						},
					},
				}
				t.Run("Pass validation", func(t *ftt.Test) {
					check := func(t testing.TB) {
						t.Helper()
						res, err := srv.ValidateConfigs(ctx, req)
						assert.Loosely(t, err, should.BeNil, truth.LineContext())
						assert.Loosely(t, res.GetMessages(), should.BeEmpty, truth.LineContext())
					}
					t.Run("With raw content", func(t *ftt.Test) {
						file.Content = &config.ValidateConfigsRequest_File_RawContent{
							RawContent: []byte("good config"),
						}
						check(t)
					})
					t.Run("With signed url", func(t *ftt.Test) {
						addFileToRemote(path, []byte("good config"), true)
						file.Content = &config.ValidateConfigsRequest_File_SignedUrl{
							SignedUrl: fmt.Sprintf("%s/%s", ts.URL, path),
						}
						check(t)
					})
				})
				t.Run("With error", func(t *ftt.Test) {
					check := func(t testing.TB) {
						t.Helper()
						res, err := srv.ValidateConfigs(ctx, req)
						assert.Loosely(t, err, should.BeNil, truth.LineContext())
						assert.Loosely(t, res, should.Resemble(&config.ValidationResult{
							Messages: []*config.ValidationResult_Message{
								{
									Path:     path,
									Text:     "in \"some_file.cfg\": blocking error",
									Severity: config.ValidationResult_ERROR,
								},
							},
						}), truth.LineContext())
					}
					t.Run("With raw content", func(t *ftt.Test) {
						file.Content = &config.ValidateConfigsRequest_File_RawContent{
							RawContent: []byte("config with error"),
						}
						check(t)
					})
					t.Run("With signed url", func(t *ftt.Test) {
						addFileToRemote(path, []byte("config with error"), true)
						file.Content = &config.ValidateConfigsRequest_File_SignedUrl{
							SignedUrl: fmt.Sprintf("%s/%s", ts.URL, path),
						}
						check(t)
					})
				})
				t.Run("With warning", func(t *ftt.Test) {
					check := func(t testing.TB) {
						t.Helper()
						res, err := srv.ValidateConfigs(ctx, req)
						assert.Loosely(t, err, should.BeNil, truth.LineContext())
						assert.Loosely(t, res, should.Resemble(&config.ValidationResult{
							Messages: []*config.ValidationResult_Message{
								{
									Path:     path,
									Text:     "in \"some_file.cfg\": diagnostic warning",
									Severity: config.ValidationResult_WARNING,
								},
							},
						}), truth.LineContext())
					}
					t.Run("With raw content", func(t *ftt.Test) {
						file.Content = &config.ValidateConfigsRequest_File_RawContent{
							RawContent: []byte("config with warning"),
						}
						check(t)
					})
					t.Run("With signed url", func(t *ftt.Test) {
						addFileToRemote(path, []byte("config with warning"), true)
						file.Content = &config.ValidateConfigsRequest_File_SignedUrl{
							SignedUrl: fmt.Sprintf("%s/%s", ts.URL, path),
						}
						check(t)
					})
				})
				t.Run("With both", func(t *ftt.Test) {
					check := func(t testing.TB) {
						t.Helper()
						res, err := srv.ValidateConfigs(ctx, req)
						assert.Loosely(t, err, should.BeNil, truth.LineContext())
						assert.Loosely(t, res, should.Resemble(&config.ValidationResult{
							Messages: []*config.ValidationResult_Message{
								{
									Path:     path,
									Text:     "in \"some_file.cfg\": blocking error",
									Severity: config.ValidationResult_ERROR,
								},
								{
									Path:     path,
									Text:     "in \"some_file.cfg\": diagnostic warning",
									Severity: config.ValidationResult_WARNING,
								},
							},
						}), truth.LineContext())
					}
					t.Run("With raw content", func(t *ftt.Test) {
						file.Content = &config.ValidateConfigsRequest_File_RawContent{
							RawContent: []byte("config with error and warning"),
						}
						check(t)
					})
					t.Run("With signed url", func(t *ftt.Test) {
						addFileToRemote(path, []byte("config with error and warning"), true)
						file.Content = &config.ValidateConfigsRequest_File_SignedUrl{
							SignedUrl: fmt.Sprintf("%s/%s", ts.URL, path),
						}
						check(t)
					})
				})

				t.Run("Signed Url not found", func(t *ftt.Test) {
					// Without adding the file to remote
					file.Content = &config.ValidateConfigsRequest_File_SignedUrl{
						SignedUrl: fmt.Sprintf("%s/%s", ts.URL, path),
					}
					res, err := srv.ValidateConfigs(ctx, req)
					grpcStatus, ok := status.FromError(err)
					assert.Loosely(t, ok, should.BeTrue)
					assert.That(t, grpcStatus.Code(), should.Equal(codes.Internal))
					assert.That(t, grpcStatus.Message(), should.ContainSubstring("Unknown resource"))
					assert.Loosely(t, res, should.BeNil)
				})
			})

			t.Run("Multiple files", func(t *ftt.Test) {
				addRule("foo.cfg")
				addRule("bar.cfg")
				addRule("baz.cfg")
				fileFoo := &config.ValidateConfigsRequest_File{
					Path: "foo.cfg",
					Content: &config.ValidateConfigsRequest_File_RawContent{
						RawContent: []byte("error"),
					},
				}
				addFileToRemote("bar.cfg", []byte("good config"), true)
				fileBar := &config.ValidateConfigsRequest_File{
					Path: "bar.cfg",
					Content: &config.ValidateConfigsRequest_File_SignedUrl{
						SignedUrl: ts.URL + "/bar.cfg",
					},
				}
				addFileToRemote("baz.cfg", []byte("warning and error"), false) // not compressed at rest
				fileBaz := &config.ValidateConfigsRequest_File{
					Path: "baz.cfg",
					Content: &config.ValidateConfigsRequest_File_SignedUrl{
						SignedUrl: ts.URL + "/baz.cfg",
					},
				}

				req := &config.ValidateConfigsRequest{
					ConfigSet: configSet,
					Files: &config.ValidateConfigsRequest_Files{
						Files: []*config.ValidateConfigsRequest_File{
							fileFoo, fileBar, fileBaz,
						},
					},
				}

				res, err := srv.ValidateConfigs(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Resemble(&config.ValidationResult{
					Messages: []*config.ValidationResult_Message{
						{
							Path:     "foo.cfg",
							Text:     "in \"foo.cfg\": blocking error",
							Severity: config.ValidationResult_ERROR,
						},
						{
							Path:     "baz.cfg",
							Text:     "in \"baz.cfg\": blocking error",
							Severity: config.ValidationResult_ERROR,
						},
						{
							Path:     "baz.cfg",
							Text:     "in \"baz.cfg\": diagnostic warning",
							Severity: config.ValidationResult_WARNING,
						},
					},
				}))
			})
		})
	})
}
