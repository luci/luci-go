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
	"net/http"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/golang/mock/gomock"

	"go.chromium.org/luci/common/gcloud/gs"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/config_service/internal/clients"
	"go.chromium.org/luci/config_service/internal/model"
	"go.chromium.org/luci/config_service/testutil"
)

func TestExamine(t *testing.T) {
	t.Parallel()

	ftt.Run("Examine", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()
		ctx = authtest.MockAuthConfig(ctx)
		ctl := gomock.NewController(t)
		mockGsClient := clients.NewMockGsClient(ctl)

		cs := config.MustProjectSet("my-project")
		const filePath = "sub/foo.cfg"
		const serviceName = "my-service"
		mockFinder := &mockFinder{
			mapping: map[string][]*model.Service{
				filePath: {
					{
						Name: serviceName,
						Info: &cfgcommonpb.Service{
							Id: serviceName,
						},
					},
				},
			},
		}
		v := &Validator{
			GsClient: mockGsClient,
			Finder:   mockFinder,
		}

		t.Run("Passed", func(t *ftt.Test) {
			mockGsClient.EXPECT().Touch(
				gomock.Any(),
				gomock.Eq("test-bucket"),
				gomock.Eq("test-object"),
			).Return(nil)

			res, err := v.Examine(ctx, cs, []File{
				testFile{
					path:   filePath,
					gsPath: gs.MakePath("test-bucket", "test-object"),
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Passed(), should.BeTrue)
		})

		t.Run("Missing file", func(t *ftt.Test) {
			mockGsClient.EXPECT().Touch(
				gomock.Any(),
				gomock.Eq("test-bucket"),
				gomock.Eq("test-object"),
			).Return(storage.ErrObjectNotExist)
			var recordedOpts *storage.SignedURLOptions
			mockGsClient.EXPECT().SignedURL(
				gomock.Eq("test-bucket"),
				gomock.Eq("test-object"),
				gomock.AssignableToTypeOf(recordedOpts),
			).DoAndReturn(
				func(_, _ string, opts *storage.SignedURLOptions) (string, error) {
					recordedOpts = opts
					return "http://example.com/singed-url", nil
				},
			)

			res, err := v.Examine(ctx, cs, []File{
				testFile{
					path:   filePath,
					gsPath: gs.MakePath("test-bucket", "test-object"),
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Passed(), should.BeFalse)
			assert.Loosely(t, res, should.Match(&ExamineResult{
				MissingFiles: []struct {
					File      File
					SignedURL string
				}{
					{
						File: testFile{
							path:   filePath,
							gsPath: gs.MakePath("test-bucket", "test-object"),
						},
						SignedURL: "http://example.com/singed-url",
					},
				},
			}))
			assert.Loosely(t, recordedOpts.Method, should.Equal(http.MethodPut))
			assert.Loosely(t, recordedOpts.Headers, should.Match([]string{"Content-Encoding:gzip", "x-goog-content-length-range:0,209715200"}))
		})

		t.Run("Unvalidatable file", func(t *ftt.Test) {
			mockFinder.mapping = nil
			res, err := v.Examine(ctx, cs, []File{
				testFile{
					path:   filePath,
					gsPath: gs.MakePath("test-bucket", "test-object"),
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Passed(), should.BeFalse)
			assert.Loosely(t, res, should.Match(&ExamineResult{
				UnvalidatableFiles: []File{
					testFile{
						path:   filePath,
						gsPath: gs.MakePath("test-bucket", "test-object"),
					},
				},
			}))
		})
	})
}
