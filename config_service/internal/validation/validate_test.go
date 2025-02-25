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

package validation

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/klauspost/compress/gzip"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/gcloud/gs"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/prpctest"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/config_service/internal/clients"
	"go.chromium.org/luci/config_service/internal/model"
	"go.chromium.org/luci/config_service/testutil"
)

func init() {
	registry.RegisterCmpOption(cmp.AllowUnexported(testFile{}))
}

type testConsumerServer struct {
	cfgcommonpb.UnimplementedConsumerServer
	fileToExpectedURL    map[string]string
	fileToValidationMsgs map[string][]*cfgcommonpb.ValidationResult_Message
	err                  error
}

func (srv *testConsumerServer) ValidateConfigs(ctx context.Context, req *cfgcommonpb.ValidateConfigsRequest) (*cfgcommonpb.ValidationResult, error) {
	if srv.err != nil {
		return nil, srv.err
	}
	result := &cfgcommonpb.ValidationResult{}
	for _, file := range req.GetFiles().GetFiles() {
		path := file.GetPath()
		switch expectedURL, ok := srv.fileToExpectedURL[path]; {
		case !ok:
			return nil, status.Errorf(codes.InvalidArgument, "unexpected file %q", path)
		case file.GetSignedUrl() != expectedURL:
			return nil, status.Errorf(codes.InvalidArgument, "expected url %q; got %q", expectedURL, file.GetSignedUrl())
		}
		switch msgs, ok := srv.fileToValidationMsgs[path]; {
		case !ok:
			return nil, status.Errorf(codes.InvalidArgument, "unexpected file %q", path)
		default:
			result.Messages = append(result.Messages, msgs...)
		}
	}

	return result, nil
}

type mockFinder struct {
	mapping map[string][]*model.Service
}

func (m *mockFinder) FindInterestedServices(_ context.Context, _ config.Set, filePath string) []*model.Service {
	return m.mapping[filePath]
}

type testFile struct {
	path    string
	gsPath  gs.Path
	content []byte
}

func (tf testFile) GetPath() string {
	return tf.path
}

func (tf testFile) GetGSPath() gs.Path {
	return tf.gsPath
}

func (tf testFile) GetRawContent(context.Context) ([]byte, error) {
	return tf.content, nil
}

var _ File = testFile{} // ensure testFile implements File interface.

func TestValidate(t *testing.T) {
	t.Parallel()

	ftt.Run("Validate", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()
		ctx = authtest.MockAuthConfig(ctx)
		ctl := gomock.NewController(t)
		mockGsClient := clients.NewMockGsClient(ctl)
		finder := &mockFinder{}
		v := &Validator{
			GsClient: mockGsClient,
			Finder:   finder,
		}

		t.Run("Single File", func(t *ftt.Test) {
			cs := config.MustProjectSet("my-project")
			const filePath = "sub/foo.cfg"
			const serviceName = "my-service"
			ts := &prpctest.Server{}
			srv := &testConsumerServer{}
			cfgcommonpb.RegisterConsumerServer(ts, srv)
			ts.Start(ctx)
			defer ts.Close()

			t.Run("No service to validate", func(t *ftt.Test) {
				res, err := v.Validate(ctx, cs, []File{testFile{path: filePath}})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Match(&cfgcommonpb.ValidationResult{}))
			})
			t.Run("Validate", func(t *ftt.Test) {
				const singedURL = "https://example.com/signed"
				var recordedOpts *storage.SignedURLOptions
				expect := func() {
					mockGsClient.EXPECT().SignedURL(
						gomock.Eq("test-bucket"),
						gomock.Eq("test-obj"),
						gomock.AssignableToTypeOf(recordedOpts),
					).DoAndReturn(
						func(_, _ string, opts *storage.SignedURLOptions) (string, error) {
							recordedOpts = opts
							return singedURL, nil
						},
					)
				}
				finder.mapping = map[string][]*model.Service{
					filePath: {
						{
							Name: serviceName,
							Info: &cfgcommonpb.Service{
								Id:       serviceName,
								Hostname: ts.Host,
							},
						},
					},
				}
				t.Run("Success", func(t *ftt.Test) {
					expect()
					srv.fileToExpectedURL = map[string]string{
						filePath: singedURL,
					}
					srv.fileToValidationMsgs = map[string][]*cfgcommonpb.ValidationResult_Message{
						filePath: {
							{
								Path:     filePath,
								Severity: cfgcommonpb.ValidationResult_ERROR,
								Text:     "bad bad bad",
							},
						},
					}

					res, err := v.Validate(ctx, cs, []File{
						testFile{path: filePath, gsPath: gs.MakePath("test-bucket", "test-obj")},
					})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res, should.Match(&cfgcommonpb.ValidationResult{
						Messages: []*cfgcommonpb.ValidationResult_Message{
							{
								Path:     filePath,
								Severity: cfgcommonpb.ValidationResult_ERROR,
								Text:     "bad bad bad",
							},
						},
					}))
					assert.Loosely(t, recordedOpts.Method, should.Equal(http.MethodGet))
					assert.Loosely(t, recordedOpts.Headers, should.BeEmpty)
				})
				t.Run("Error", func(t *ftt.Test) {
					expect()
					srv.err = status.Errorf(codes.Internal, "internal server error")

					res, err := v.Validate(ctx, cs, []File{
						testFile{path: filePath, gsPath: gs.MakePath("test-bucket", "test-obj")},
					})
					assert.Loosely(t, err, should.ErrLike("failed to validate configs against service \"my-service\""))
					assert.Loosely(t, res, should.BeNil)
				})
			})

			t.Run("Validate against self", func(t *ftt.Test) {
				v.SelfRuleSet = validation.NewRuleSet()
				finder.mapping = map[string][]*model.Service{
					filePath: {
						{
							Name: testutil.AppID,
							Info: &cfgcommonpb.Service{
								Id:       testutil.AppID,
								Hostname: ts.Host,
							},
						},
					},
				}
				t.Run("Succeeds", func(t *ftt.Test) {
					var validated bool
					var recordedContent []byte
					v.SelfRuleSet.Add(string(cs), filePath, func(vCtx *validation.Context, configSet, path string, content []byte) error {
						validated = true
						recordedContent = content
						vCtx.Errorf("bad config")
						return nil
					})
					tf := testFile{
						path:    filePath,
						content: []byte("This is config content"),
					}
					res, err := v.Validate(ctx, cs, []File{tf})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res, should.Match(&cfgcommonpb.ValidationResult{
						Messages: []*cfgcommonpb.ValidationResult_Message{
							{
								Path:     filePath,
								Severity: cfgcommonpb.ValidationResult_ERROR,
								Text:     "in \"sub/foo.cfg\": bad config",
							},
						},
					}))
					assert.Loosely(t, validated, should.BeTrue)
					assert.Loosely(t, recordedContent, should.Match(tf.content))
				})
				t.Run("Error", func(t *ftt.Test) {
					v.SelfRuleSet.Add(string(cs), filePath, func(vCtx *validation.Context, configSet, path string, content []byte) error {
						return errors.New("something went wrong")
					})
					tf := testFile{
						path:    filePath,
						content: []byte("This is config content"),
					}
					res, err := v.Validate(ctx, cs, []File{tf})
					assert.Loosely(t, err, should.ErrLike("something went wrong"))
					assert.Loosely(t, res, should.BeNil)
				})
			})
		})

		t.Run("Multiple files and services", func(t *ftt.Test) {
			// test cases:
			//  4 files: a,b,c,d and 2 services: foo and bar
			//  file a: validated by both foo and bar, foo output 1 warning and bar
			//          output 1 error.
			//  file b: validated by foo, no error or warning
			//  file c: validated by bar, bar returns 1 warning and 1 error.
			//  file d: no service can validate file d
			fileA := testFile{
				path:   "a.cfg",
				gsPath: gs.MakePath("test-bucket", "test-object-a"),
			}
			fileB := testFile{
				path:   "b.cfg",
				gsPath: gs.MakePath("test-bucket", "test-object-b"),
			}
			fileC := testFile{
				path:   "c.cfg",
				gsPath: gs.MakePath("test-bucket", "test-object-c"),
			}
			fileD := testFile{
				path:   "d.cfg",
				gsPath: gs.MakePath("test-bucket", "test-object-d"),
			}

			testServerFoo := &prpctest.Server{}
			consumerServerFoo := &testConsumerServer{}
			cfgcommonpb.RegisterConsumerServer(testServerFoo, consumerServerFoo)
			testServerFoo.Start(ctx)
			defer testServerFoo.Close()

			testServerBar := &prpctest.Server{}
			consumerServerBar := &testConsumerServer{}
			cfgcommonpb.RegisterConsumerServer(testServerBar, consumerServerBar)
			testServerBar.Start(ctx)
			defer testServerBar.Close()

			serviceFoo := &model.Service{
				Name: "foo",
				Info: &cfgcommonpb.Service{
					Id:       "foo",
					Hostname: testServerFoo.Host,
				},
			}
			serviceBar := &model.Service{
				Name: "bar",
				Info: &cfgcommonpb.Service{
					Id:       "bar",
					Hostname: testServerBar.Host,
				},
			}

			const signedURLPrefix = "https://example.com/signed"
			mockGsClient.EXPECT().SignedURL(
				gomock.Eq("test-bucket"),
				gomock.Any(),
				gomock.Any(),
			).DoAndReturn(
				func(bucket, object string, _ *storage.SignedURLOptions) (string, error) {
					return fmt.Sprintf("%s/%s/%s", signedURLPrefix, bucket, object), nil
				},
			).AnyTimes()

			finder.mapping = map[string][]*model.Service{
				fileA.path: {serviceFoo, serviceBar},
				fileB.path: {serviceFoo},
				fileC.path: {serviceBar},
				// No service can validate fileD.
			}

			consumerServerFoo.fileToExpectedURL = map[string]string{
				fileA.path: "https://example.com/signed/test-bucket/test-object-a",
				fileB.path: "https://example.com/signed/test-bucket/test-object-b",
			}
			consumerServerBar.fileToExpectedURL = map[string]string{
				fileA.path: "https://example.com/signed/test-bucket/test-object-a",
				fileC.path: "https://example.com/signed/test-bucket/test-object-c",
			}
			consumerServerFoo.fileToValidationMsgs = map[string][]*cfgcommonpb.ValidationResult_Message{
				fileA.path: {
					{
						Path:     fileA.path,
						Severity: cfgcommonpb.ValidationResult_WARNING,
						Text:     "warning for file a from service foo",
					},
				},
				fileB.path: {}, // No validation error for fileB
			}
			consumerServerBar.fileToValidationMsgs = map[string][]*cfgcommonpb.ValidationResult_Message{
				fileA.path: {
					{
						Path:     fileA.path,
						Severity: cfgcommonpb.ValidationResult_ERROR,
						Text:     "error for file a from service bar",
					},
				},
				fileC.path: {
					{
						Path:     fileC.path,
						Severity: cfgcommonpb.ValidationResult_WARNING,
						Text:     "warning for file c from service bar",
					},
					{
						Path:     fileC.path,
						Severity: cfgcommonpb.ValidationResult_ERROR,
						Text:     "error for file c from service bar",
					},
				},
			}

			res, err := v.Validate(ctx, config.MustProjectSet("my-project"), []File{fileA, fileB, fileC, fileD})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Match(&cfgcommonpb.ValidationResult{
				Messages: []*cfgcommonpb.ValidationResult_Message{
					{
						Path:     fileA.path,
						Severity: cfgcommonpb.ValidationResult_ERROR,
						Text:     "error for file a from service bar",
					},
					{
						Path:     fileA.path,
						Severity: cfgcommonpb.ValidationResult_WARNING,
						Text:     "warning for file a from service foo",
					},
					{
						Path:     fileC.path,
						Severity: cfgcommonpb.ValidationResult_ERROR,
						Text:     "error for file c from service bar",
					},
					{
						Path:     fileC.path,
						Severity: cfgcommonpb.ValidationResult_WARNING,
						Text:     "warning for file c from service bar",
					},
				},
			}))
		})
	})
}

func TestValidateLegacy(t *testing.T) {
	t.Parallel()

	ftt.Run("Validate using legacy protocol", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()
		ctx = authtest.MockAuthConfig(ctx)
		ctl := gomock.NewController(t)
		mockGsClient := clients.NewMockGsClient(ctl)
		finder := &mockFinder{}
		v := &Validator{
			GsClient: mockGsClient,
			Finder:   finder,
		}

		var srvResponse []byte
		var srvErrMsg string
		var capturedRequestBody []byte
		var capturedRequestHeader http.Header
		legacyTestSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedRequestHeader = r.Header
			var err error
			if capturedRequestBody, err = io.ReadAll(r.Body); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "%s", err)
				return
			}
			if srvErrMsg != "" {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, srvErrMsg)
				return
			}
			if _, err := w.Write(srvResponse); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "failed to write response: %s", err)
			}
		}))
		defer legacyTestSrv.Close()

		cs := config.MustProjectSet("my-project")
		const filePath = "sub/foo.cfg"
		const serviceName = "my-service"
		finder.mapping = map[string][]*model.Service{
			filePath: {
				{
					Name: serviceName,
					Info: &cfgcommonpb.Service{
						Id:          serviceName,
						MetadataUrl: legacyTestSrv.URL,
					},
					LegacyMetadata: &cfgcommonpb.ServiceDynamicMetadata{
						Version: "1.0",
						Validation: &cfgcommonpb.Validator{
							Url: legacyTestSrv.URL,
							Patterns: []*cfgcommonpb.ConfigPattern{
								{ConfigSet: string(cs), Path: filePath},
							},
						},
						SupportsGzipCompression: true,
					},
				},
			},
		}

		t.Run("Works", func(t *ftt.Test) {
			check := func(t testing.TB) {
				t.Helper()
				tf := testFile{
					path:    filePath,
					content: []byte("This is config content"),
				}
				res, err := v.Validate(ctx, cs, []File{tf})
				assert.Loosely(t, err, should.BeNil, truth.LineContext())
				assert.Loosely(t, res, should.Match(&cfgcommonpb.ValidationResult{
					Messages: []*cfgcommonpb.ValidationResult_Message{
						{
							Path:     filePath,
							Severity: cfgcommonpb.ValidationResult_ERROR,
							Text:     "bad config",
						},
					},
				}), truth.LineContext())

				assert.Loosely(t, capturedRequestBody, should.NotBeEmpty, truth.LineContext())
				reqMap := map[string]any{}
				assert.Loosely(t, json.Unmarshal(capturedRequestBody, &reqMap), should.BeNil, truth.LineContext())
				assert.Loosely(t, reqMap, should.HaveLength(3), truth.LineContext())
				assert.Loosely(t, reqMap["config_set"], should.Equal("projects/my-project"), truth.LineContext())
				assert.Loosely(t, reqMap["path"], should.Equal(filePath), truth.LineContext())
				assert.Loosely(t, reqMap["content"], should.Equal(base64.StdEncoding.EncodeToString(tf.content)), truth.LineContext())
				assert.Loosely(t, capturedRequestHeader.Get("Content-Type"), should.Equal("application/json; charset=utf-8"), truth.LineContext())
				assert.Loosely(t, capturedRequestHeader.Get("Content-Encoding"), should.BeEmpty, truth.LineContext())
			}

			t.Run("With int severity", func(t *ftt.Test) {
				srvResponse = []byte(`{"messages": [{"severity": 40, "text": "bad config"}]}`)
				check(t)
			})
			t.Run("With string severity", func(t *ftt.Test) {
				srvResponse = []byte(`{"messages": [{"severity": "ERROR", "text": "bad config"}]}`)
				check(t)
			})
		})

		t.Run("Empty messages", func(t *ftt.Test) {
			srvResponse = []byte(`{"messages": []}`)
			tf := testFile{
				path:    filePath,
				content: []byte("This is config content"),
			}
			res, err := v.Validate(ctx, cs, []File{tf})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Match(&cfgcommonpb.ValidationResult{}))
		})

		t.Run("Empty response", func(t *ftt.Test) {
			srvResponse = nil
			tf := testFile{
				path:    filePath,
				content: []byte("This is config content"),
			}
			res, err := v.Validate(ctx, cs, []File{tf})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Match(&cfgcommonpb.ValidationResult{}))
		})

		t.Run("Compress large payload", func(t *ftt.Test) {
			tf := testFile{
				path:    filePath,
				content: make([]byte, 1024*1024),
			}
			_, err := rand.Read(tf.content)
			assert.Loosely(t, err, should.BeNil)
			res, err := v.Validate(ctx, cs, []File{tf})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.NotBeNil)

			assert.Loosely(t, capturedRequestBody, should.NotBeEmpty)
			r, err := gzip.NewReader(bytes.NewBuffer(capturedRequestBody))
			assert.Loosely(t, err, should.BeNil)
			uncompressed, err := io.ReadAll(r)
			assert.Loosely(t, err, should.BeNil)
			reqMap := map[string]any{}
			assert.Loosely(t, json.Unmarshal(uncompressed, &reqMap), should.BeNil)
			assert.Loosely(t, reqMap, should.HaveLength(3))
			assert.Loosely(t, reqMap["content"], should.Equal(base64.StdEncoding.EncodeToString(tf.content)))
			assert.Loosely(t, capturedRequestHeader.Get("Content-Type"), should.Equal("application/json; charset=utf-8"))
			assert.Loosely(t, capturedRequestHeader.Get("Content-Encoding"), should.Equal("gzip"))
		})

		t.Run("Omit unknown severity", func(t *ftt.Test) {
			t.Run("Not provided", func(t *ftt.Test) {
				srvResponse = []byte(`{"messages": [{"text": "bad config"}]}`)
			})
			t.Run("Returns unknown", func(t *ftt.Test) {
				srvResponse = []byte(`{"messages": [{"severity": 0, "text": "bad config"}]}`)
			})

			tf := testFile{
				path:    filePath,
				content: []byte("This is config content"),
			}
			res, err := v.Validate(ctx, cs, []File{tf})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Match(&cfgcommonpb.ValidationResult{}))
		})

		t.Run("Error on unrecognized severity", func(t *ftt.Test) {
			check := func(t testing.TB) {
				t.Helper()
				tf := testFile{
					path:    filePath,
					content: []byte("This is config content"),
				}
				res, err := v.Validate(ctx, cs, []File{tf})
				assert.Loosely(t, err, should.ErrLike("unrecognized severity"), truth.LineContext())
				assert.Loosely(t, res, should.BeNil, truth.LineContext())
			}

			t.Run("Int severity", func(t *ftt.Test) {
				srvResponse = []byte(`{"messages": [{"severity": 1234, "text": "bad config"}]}`)
				check(t)
			})
			t.Run("String severity", func(t *ftt.Test) {
				srvResponse = []byte(`{"messages": [{"severity": "BAD", "text": "bad config"}]}`)
				check(t)
			})
			t.Run("Not int not string", func(t *ftt.Test) {
				srvResponse = []byte(`{"messages": [{"severity": true, "text": "bad config"}]}`)
				check(t)
			})
		})

		t.Run("Server Error", func(t *ftt.Test) {
			srvErrMsg = "server encounter error"
			tf := testFile{
				path:    filePath,
				content: []byte("This is config content"),
			}
			res, err := v.Validate(ctx, cs, []File{tf})
			assert.Loosely(t, err, should.ErrLike(legacyTestSrv.URL+" returns 500"))
			assert.Loosely(t, res, should.BeNil)
		})

		t.Run("Server returns malformed response", func(t *ftt.Test) {
			srvResponse = []byte("[")
			tf := testFile{
				path:    filePath,
				content: []byte("This is config content"),
			}
			res, err := v.Validate(ctx, cs, []File{tf})
			assert.Loosely(t, err, should.ErrLike("failed to unmarshal response"))
			assert.Loosely(t, res, should.BeNil)
		})
	})
}
