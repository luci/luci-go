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

package sink

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestValidateTestResult(t *testing.T) {
	t.Parallel()
	ftt.Run(`ValidateTestResult`, t, func(t *ftt.Test) {
		tr := validTestResult(t)

		t.Run(`TestLocation`, func(t *ftt.Test) {
			tr.TestMetadata.Location.FileName = ""
			err := validateTestResult(testclock.TestRecentTimeUTC, tr)
			assert.Loosely(t, err, should.ErrLike("test_metadata: location: file_name: unspecified"))
		})

		t.Run(`TestMetadata`, func(t *ftt.Test) {
			tr.TestMetadata = &pb.TestMetadata{
				Name: "name",
				Location: &pb.TestLocation{
					Repo:     "https://chromium.googlesource.com/chromium/src",
					FileName: "//file",
				},
			}
			err := validateTestResult(testclock.TestRecentTimeUTC, tr)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`Properties`, func(t *ftt.Test) {
			t.Run(`Valid Properties`, func(t *ftt.Test) {
				tr.Properties = &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"key_1": structpb.NewStringValue("value_1"),
						"key_2": structpb.NewStructValue(&structpb.Struct{
							Fields: map[string]*structpb.Value{
								"child_key": structpb.NewNumberValue(1),
							},
						}),
					},
				}
				err := validateTestResult(testclock.TestRecentTimeUTC, tr)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run(`Large Properties`, func(t *ftt.Test) {
				tr.Properties = &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"key1": structpb.NewStringValue(strings.Repeat("1", pbutil.MaxSizeTestResultProperties)),
					},
				}
				err := validateTestResult(testclock.TestRecentTimeUTC, tr)
				assert.Loosely(t, err, should.ErrLike(`properties: exceeds the maximum size of`))
				assert.Loosely(t, err, should.ErrLike(`bytes`))
			})
		})
	})
}

func TestValidateArtifacts(t *testing.T) {
	t.Parallel()
	// valid artifacts
	validArts := map[string]*sinkpb.Artifact{
		"art1": {
			Body:        &sinkpb.Artifact_FilePath{FilePath: "/tmp/foo"},
			ContentType: "text/plain",
		},
		"art2": {
			Body:        &sinkpb.Artifact_Contents{Contents: []byte("contents")},
			ContentType: "text/plain",
		},
		"art3": {
			Body:        &sinkpb.Artifact_GcsUri{GcsUri: "gs://bucket/bar"},
			ContentType: "text/plain",
		},
	}
	// invalid artifacts
	invalidArts := map[string]*sinkpb.Artifact{
		"art1": {ContentType: "text/plain"},
	}

	ftt.Run("Succeeds", t, func(t *ftt.Test) {
		t.Run("with no artifact", func(t *ftt.Test) {
			assert.Loosely(t, validateArtifacts(nil), should.BeNil)
		})

		t.Run("with valid artifacts", func(t *ftt.Test) {
			assert.Loosely(t, validateArtifacts(validArts), should.BeNil)
		})
	})

	ftt.Run("Fails", t, func(t *ftt.Test) {
		expected := "body: one of file_path or contents or gcs_uri must be provided"

		t.Run("with invalid artifacts", func(t *ftt.Test) {
			assert.Loosely(t, validateArtifacts(invalidArts), should.ErrLike(expected))
		})

		t.Run("with a mix of valid and invalid artifacts", func(t *ftt.Test) {
			invalidArts["art2"] = validArts["art2"]
			assert.Loosely(t, validateArtifacts(invalidArts), should.ErrLike(expected))
		})
	})
}
