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
	"errors"
	"testing"

	"go.chromium.org/luci/common/gcloud/gs"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/config_service/internal/model"
	"go.chromium.org/luci/config_service/testutil"
)

func TestExamine(t *testing.T) {
	t.Parallel()

	ftt.Run("Examine", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()
		ctx = authtest.MockAuthConfig(ctx)

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
			Finder: mockFinder,
		}

		t.Run("Passed", func(t *ftt.Test) {
			res, err := v.Examine(ctx, cs, []File{
				testFile{
					path:   filePath,
					gsPath: gs.MakePath("test-bucket", "test-object"),
					putURL: "",
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Passed(), should.BeTrue)
		})

		t.Run("Missing file", func(t *ftt.Test) {
			tf := testFile{
				path:   filePath,
				gsPath: gs.MakePath("test-bucket", "test-object"),
				putURL: "http://example.com/signed-url",
			}
			res, err := v.Examine(ctx, cs, []File{tf})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Passed(), should.BeFalse)
			assert.Loosely(t, res, should.Match(&ExamineResult{
				MissingFiles: []struct {
					File      File
					SignedURL string
				}{
					{
						File:      tf,
						SignedURL: "http://example.com/signed-url",
					},
				},
			}))
		})

		t.Run("Unvalidatable file", func(t *ftt.Test) {
			mockFinder.mapping = nil
			tf := testFile{
				path:   filePath,
				gsPath: gs.MakePath("test-bucket", "test-object"),
				putURL: "http://example.com/should-not-be-used",
			}
			res, err := v.Examine(ctx, cs, []File{tf})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Passed(), should.BeFalse)
			assert.Loosely(t, res, should.Match(&ExamineResult{
				UnvalidatableFiles: []File{tf},
			}))
		})
	})
}

func TestExaminePutURLError(t *testing.T) {
	t.Parallel()

	ctx := testutil.SetupContext()
	cs := config.MustProjectSet("my-project")
	const filePath = "sub/foo.cfg"
	v := &Validator{
		Finder: &mockFinder{
			mapping: map[string][]*model.Service{
				filePath: {{Name: "svc", Info: &cfgcommonpb.Service{Id: "svc"}}},
			},
		},
	}

	res, err := v.Examine(ctx, cs, []File{
		testFile{
			path:   filePath,
			gsPath: gs.MakePath("test-bucket", "test-object"),
			putErr: errors.New("put url error"),
		},
	})
	assert.ErrIsLike(t, err, "put url error")
	assert.That(t, res == nil, should.BeTrue)
}
