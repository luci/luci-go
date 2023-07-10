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
	"context"
	"fmt"
	"net/http"
	"testing"

	"cloud.google.com/go/storage"
	"go.chromium.org/luci/common/gcloud/gs"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/testing/prpctest"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/server/auth/authtest"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/config_service/internal/clients"
	"go.chromium.org/luci/config_service/internal/model"
	"go.chromium.org/luci/config_service/testutil"

	"github.com/golang/mock/gomock"
	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

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

func (m *mockFinder) FindInterestedServices(_ config.Set, filePath string) []*model.Service {
	return m.mapping[filePath]
}

type testFile struct {
	path   string
	gsPath gs.Path
}

func (tf testFile) GetPath() string {
	return tf.path
}

func (tf testFile) GetGSPath() gs.Path {
	return tf.gsPath
}

var _ File = testFile{} // ensure testFile implements File interface.

func TestValidate(t *testing.T) {
	t.Parallel()

	Convey("Validate", t, func() {
		ctx := testutil.SetupContext()
		ctx = authtest.MockAuthConfig(ctx)
		ctl := gomock.NewController(t)
		mockGsClient := clients.NewMockGsClient(ctl)
		finder := &mockFinder{}
		v := &Validator{
			GsClient: mockGsClient,
			Finder:   finder,
		}

		Convey("Single File", func() {
			cs := config.MustProjectSet("my-project")
			const filePath = "sub/foo.cfg"
			const serviceName = "my-service"
			ts := &prpctest.Server{}
			srv := &testConsumerServer{}
			cfgcommonpb.RegisterConsumerServer(ts, srv)
			ts.Start(ctx)
			defer ts.Close()

			Convey("No service to validate", func() {
				res, err := v.Validate(ctx, cs, []File{testFile{path: filePath}})
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &cfgcommonpb.ValidationResult{})
			})
			Convey("Validate", func() {
				const singedURL = "https://example.com/signed"
				var recordedOpts *storage.SignedURLOptions
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

				finder.mapping = map[string][]*model.Service{
					filePath: {
						{
							Name: serviceName,
							Info: &cfgcommonpb.Service{
								Id:              serviceName,
								ServiceEndpoint: ts.Host,
							},
						},
					},
				}
				Convey("Success", func() {
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
					So(err, ShouldBeNil)
					So(res, ShouldResembleProto, &cfgcommonpb.ValidationResult{
						Messages: []*cfgcommonpb.ValidationResult_Message{
							{
								Path:     filePath,
								Severity: cfgcommonpb.ValidationResult_ERROR,
								Text:     "bad bad bad",
							},
						},
					})
					So(recordedOpts.Method, ShouldEqual, http.MethodGet)
					So(recordedOpts.Headers, ShouldResemble, []string{"Accept-Encoding:gzip"})
				})
				Convey("Error", func() {
					srv.err = status.Errorf(codes.Internal, "internal server error")

					res, err := v.Validate(ctx, cs, []File{
						testFile{path: filePath, gsPath: gs.MakePath("test-bucket", "test-obj")},
					})
					So(err, ShouldErrLike, "failed to validate configs against service \"my-service\"")
					So(res, ShouldBeNil)
				})
			})
		})

		Convey("Multiple files and services", func() {
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
					Id:              "foo",
					ServiceEndpoint: testServerFoo.Host,
				},
			}
			serviceBar := &model.Service{
				Name: "bar",
				Info: &cfgcommonpb.Service{
					Id:              "bar",
					ServiceEndpoint: testServerBar.Host,
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
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &cfgcommonpb.ValidationResult{
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
			})
		})
	})
}
