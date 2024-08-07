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
	"encoding/json"
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
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestInstallHandlers(t *testing.T) {
	t.Parallel()

	Convey("Initialization of validator, validation routes and handlers", t, func() {
		rules := validation.NewRuleSet()

		r := router.New()
		rr := httptest.NewRecorder()
		host := "example.com"

		metaCall := func() *config.ServiceDynamicMetadata {
			req, err := http.NewRequest("GET", "https://"+host+metadataPath, nil)
			So(err, ShouldBeNil)
			r.ServeHTTP(rr, req)

			var resp config.ServiceDynamicMetadata
			err = json.NewDecoder(rr.Body).Decode(&resp)
			So(err, ShouldBeNil)
			return &resp
		}
		valCall := func(configSet, path, content string) *config.ValidationResponseMessage {
			respBodyJSON, err := json.Marshal(config.ValidationRequestMessage{
				ConfigSet: configSet,
				Path:      path,
				Content:   []byte(content),
			})
			So(err, ShouldBeNil)
			req, err := http.NewRequest("POST", validationPath, bytes.NewReader(respBodyJSON))
			So(err, ShouldBeNil)
			r.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				return nil
			}
			var resp config.ValidationResponseMessage
			err = json.NewDecoder(rr.Body).Decode(&resp)
			So(err, ShouldBeNil)
			return &resp
		}

		InstallHandlers(r, nil, rules)

		Convey("Basic metadataHandler call", func() {
			So(rr.Code, ShouldEqual, http.StatusOK)
			So(metaCall(), ShouldResemble, &config.ServiceDynamicMetadata{
				Version:                 metaDataFormatVersion,
				SupportsGzipCompression: true,
				Validation: &config.Validator{
					Url: fmt.Sprintf("https://%s%s", host, validationPath),
				},
			})
		})

		Convey("metadataHandler call with patterns", func() {
			rules.Add("configSet", "path", nil)
			meta := metaCall()
			So(rr.Code, ShouldEqual, http.StatusOK)
			So(meta, ShouldResemble, &config.ServiceDynamicMetadata{
				Version:                 metaDataFormatVersion,
				SupportsGzipCompression: true,
				Validation: &config.Validator{
					Url: fmt.Sprintf("https://%s%s", host, validationPath),
					Patterns: []*config.ConfigPattern{
						{
							ConfigSet: "exact:configSet",
							Path:      "exact:path",
						},
					},
				},
			})
		})

		Convey("Basic validationHandler call", func() {
			rules.Add("dead", "beef", func(ctx *validation.Context, configSet, path string, content []byte) error {
				So(string(content), ShouldEqual, "content")
				ctx.Errorf("blocking error")
				ctx.Warningf("diagnostic warning")
				return nil
			})
			valResp := valCall("dead", "beef", "content")
			So(rr.Code, ShouldEqual, http.StatusOK)
			So(valResp, ShouldResemble, &config.ValidationResponseMessage{
				Messages: []*config.ValidationResponseMessage_Message{
					{
						Text:     "in \"beef\": blocking error",
						Severity: config.ValidationResponseMessage_ERROR,
					},
					{
						Text:     "in \"beef\": diagnostic warning",
						Severity: config.ValidationResponseMessage_WARNING,
					},
				},
			})
		})

		Convey("validationHandler call with no configSet or path", func() {
			valCall("", "", "")
			So(rr.Code, ShouldEqual, http.StatusBadRequest)
			So(rr.Body.String(), ShouldEqual, "Must specify the config_set of the file to validate")
		})

		Convey("validationHandler call with no path", func() {
			valCall("dead", "", "")
			So(rr.Code, ShouldEqual, http.StatusBadRequest)
			So(rr.Body.String(), ShouldEqual, "Must specify the path of the file to validate")
		})
	})
}

func TestConsumerServer(t *testing.T) {
	t.Parallel()

	Convey("ConsumerServer", t, func() {
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

		Convey("Check caller", func() {
			Convey("Allow LUCI Config service account", func() {
				_, err := srv.GetMetadata(ctx, &emptypb.Empty{})
				So(err, ShouldBeNil)
			})
			Convey("Allow Admin group", func() {
				authState := &authtest.FakeState{
					Identity:       "user:someone@example.com",
					IdentityGroups: []string{adminGroup},
				}
				ctx = auth.WithState(ctx, authState)
				_, err := srv.GetMetadata(ctx, &emptypb.Empty{})
				So(err, ShouldBeNil)
			})
			Convey("Disallow", func() {
				Convey("Non-admin users", func() {
					authState = &authtest.FakeState{
						Identity: "user:someone@example.com",
					}
				})
				Convey("Anonymous", func() {
					authState = &authtest.FakeState{
						Identity: identity.AnonymousIdentity,
					}
				})
				ctx = auth.WithState(ctx, authState)
				_, err := srv.GetMetadata(ctx, &emptypb.Empty{})
				So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
			})
		})

		Convey("GetMetadata", func() {
			rules.Add("configSet", "path", nil)
			res, err := srv.GetMetadata(ctx, &emptypb.Empty{})
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &config.ServiceMetadata{
				ConfigPatterns: []*config.ConfigPattern{
					{
						ConfigSet: "exact:configSet",
						Path:      "exact:path",
					},
				},
			})
		})

		Convey("ValidateConfig", func() {
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
					So(err, ShouldBeNil)
					So(gw.Close(), ShouldBeNil)
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

			Convey("Single file", func() {
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
				Convey("Pass validation", func() {
					Convey("With raw content", func() {
						file.Content = &config.ValidateConfigsRequest_File_RawContent{
							RawContent: []byte("good config"),
						}
					})
					Convey("With signed url", func() {
						addFileToRemote(path, []byte("good config"), true)
						file.Content = &config.ValidateConfigsRequest_File_SignedUrl{
							SignedUrl: fmt.Sprintf("%s/%s", ts.URL, path),
						}
					})
					res, err := srv.ValidateConfigs(ctx, req)
					So(err, ShouldBeNil)
					So(res.GetMessages(), ShouldBeEmpty)
				})
				Convey("With error", func() {
					Convey("With raw content", func() {
						file.Content = &config.ValidateConfigsRequest_File_RawContent{
							RawContent: []byte("config with error"),
						}
					})
					Convey("With signed url", func() {
						addFileToRemote(path, []byte("config with error"), true)
						file.Content = &config.ValidateConfigsRequest_File_SignedUrl{
							SignedUrl: fmt.Sprintf("%s/%s", ts.URL, path),
						}
					})
					res, err := srv.ValidateConfigs(ctx, req)
					So(err, ShouldBeNil)
					So(res, ShouldResembleProto, &config.ValidationResult{
						Messages: []*config.ValidationResult_Message{
							{
								Path:     path,
								Text:     "in \"some_file.cfg\": blocking error",
								Severity: config.ValidationResult_ERROR,
							},
						},
					})
				})
				Convey("With warning", func() {
					Convey("With raw content", func() {
						file.Content = &config.ValidateConfigsRequest_File_RawContent{
							RawContent: []byte("config with warning"),
						}
					})
					Convey("With signed url", func() {
						addFileToRemote(path, []byte("config with warning"), true)
						file.Content = &config.ValidateConfigsRequest_File_SignedUrl{
							SignedUrl: fmt.Sprintf("%s/%s", ts.URL, path),
						}
					})
					res, err := srv.ValidateConfigs(ctx, req)
					So(err, ShouldBeNil)
					So(res, ShouldResembleProto, &config.ValidationResult{
						Messages: []*config.ValidationResult_Message{
							{
								Path:     path,
								Text:     "in \"some_file.cfg\": diagnostic warning",
								Severity: config.ValidationResult_WARNING,
							},
						},
					})
				})
				Convey("With both", func() {
					Convey("With raw content", func() {
						file.Content = &config.ValidateConfigsRequest_File_RawContent{
							RawContent: []byte("config with error and warning"),
						}
					})
					Convey("With signed url", func() {
						addFileToRemote(path, []byte("config with error and warning"), true)
						file.Content = &config.ValidateConfigsRequest_File_SignedUrl{
							SignedUrl: fmt.Sprintf("%s/%s", ts.URL, path),
						}
					})
					res, err := srv.ValidateConfigs(ctx, req)
					So(err, ShouldBeNil)
					So(res, ShouldResembleProto, &config.ValidationResult{
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
					})
				})

				Convey("Signed Url not found", func() {
					// Without adding the file to remote
					file.Content = &config.ValidateConfigsRequest_File_SignedUrl{
						SignedUrl: fmt.Sprintf("%s/%s", ts.URL, path),
					}
					res, err := srv.ValidateConfigs(ctx, req)
					grpcStatus, ok := status.FromError(err)
					So(ok, ShouldBeTrue)
					So(grpcStatus, ShouldBeLikeStatus, codes.Internal, "Unknown resource")
					So(res, ShouldBeNil)
				})
			})

			Convey("Multiple files", func() {
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
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &config.ValidationResult{
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
				})
			})
		})
	})
}
