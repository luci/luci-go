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

package pbutil

import (
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestParseArtifactName(t *testing.T) {
	t.Parallel()
	ftt.Run(`ParseArtifactName`, t, func(t *ftt.Test) {
		t.Run(`Work unit level`, func(t *ftt.Test) {
			t.Run(`Success`, func(t *ftt.Test) {
				rootInvocationID, workUnitID, testID, resultID, artifactID, err := ParseArtifactName("rootInvocations/inv/workUnits/wu/artifacts/a")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rootInvocationID, should.Equal("inv"))
				assert.Loosely(t, workUnitID, should.Equal("wu"))
				assert.Loosely(t, testID, should.BeEmpty)
				assert.Loosely(t, resultID, should.BeEmpty)
				assert.Loosely(t, artifactID, should.Equal("a"))
			})
			t.Run(`With a slash`, func(t *ftt.Test) {
				_, _, _, _, artifactID, err := ParseArtifactName("rootInvocations/inv/workUnits/wu/artifacts/a%2Fb")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, artifactID, should.Equal("a/b"))
			})
			t.Run(`With a percent sign`, func(t *ftt.Test) {
				_, _, _, _, artifactID, err := ParseArtifactName("rootInvocations/inv/workUnits/wu/artifacts/a%25b")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, artifactID, should.Equal("a%b"))
			})
			t.Run(`Success with a long artifact name`, func(t *ftt.Test) {
				artName := strings.Repeat("a%2Fb", 100) // 500 characters
				wantArtID := strings.Repeat("a/b", 100)
				_, _, _, _, gotArtID, err := ParseArtifactName("rootInvocations/inv/workUnits/wu/artifacts/" + artName)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, gotArtID, should.Equal(wantArtID))
			})
			t.Run(`Failure with a long artifact name over the character limit`, func(t *ftt.Test) {
				artName := strings.Repeat("a", 600)
				_, _, _, _, _, err := ParseArtifactName("rootInvocations/inv/workUnits/wu/artifacts/" + artName)
				assert.Loosely(t, err, should.NotBeNil)
			})
		})
		t.Run(`Test result level`, func(t *ftt.Test) {
			t.Run(`Success`, func(t *ftt.Test) {
				rootInvocationID, workUnitID, testID, resultID, artifactID, err := ParseArtifactName("rootInvocations/inv/workUnits/wu/tests/t/results/r/artifacts/a")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rootInvocationID, should.Equal("inv"))
				assert.Loosely(t, workUnitID, should.Equal("wu"))
				assert.Loosely(t, testID, should.Equal("t"))
				assert.Loosely(t, resultID, should.Equal("r"))
				assert.Loosely(t, artifactID, should.Equal("a"))
			})
			t.Run(`With a slash in test ID`, func(t *ftt.Test) {
				_, _, testID, _, _, err := ParseArtifactName("rootInvocations/inv/workUnits/wu/tests/t%2F/results/r/artifacts/a/b")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, testID, should.Equal("t/"))
			})
			t.Run(`With a slash`, func(t *ftt.Test) {
				_, _, _, _, artifactID, err := ParseArtifactName("rootInvocations/inv/workUnits/wu/tests/t/results/r/artifacts/a%2Fb")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, artifactID, should.Equal("a/b"))
			})
		})
	})
}

func TestParseLegacyArtifactName(t *testing.T) {
	t.Parallel()
	ftt.Run(`ParseArtifactName`, t, func(t *ftt.Test) {
		t.Run(`Invocation level`, func(t *ftt.Test) {
			t.Run(`Success`, func(t *ftt.Test) {
				invocationID, testID, resultID, artifactID, err := ParseLegacyArtifactName("invocations/inv/artifacts/a")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, invocationID, should.Equal("inv"))
				assert.Loosely(t, testID, should.BeEmpty)
				assert.Loosely(t, resultID, should.BeEmpty)
				assert.Loosely(t, artifactID, should.Equal("a"))
			})

			t.Run(`With a slash`, func(t *ftt.Test) {
				_, _, _, artifactID, err := ParseLegacyArtifactName("invocations/inv/artifacts/a%2Fb")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, artifactID, should.Equal("a/b"))
			})

			t.Run(`With a percent sign`, func(t *ftt.Test) {
				_, _, _, artifactID, err := ParseLegacyArtifactName("invocations/inv/artifacts/a%25b")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, artifactID, should.Equal("a%b"))
			})

			t.Run(`Success with a long artifact name`, func(t *ftt.Test) {
				artName := strings.Repeat("a%2Fb", 100) // 500 characters
				wantArtID := strings.Repeat("a/b", 100)
				_, _, _, gotArtID, err := ParseLegacyArtifactName("invocations/inv/artifacts/" + artName)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, gotArtID, should.Equal(wantArtID))
			})

			t.Run(`Failure with a long artifact name over the character limit`, func(t *ftt.Test) {
				artName := strings.Repeat("a", 600)
				_, _, _, _, err := ParseLegacyArtifactName("invocations/inv/artifacts/" + artName)
				assert.Loosely(t, err, should.NotBeNil)
			})
		})

		t.Run(`Test result level`, func(t *ftt.Test) {
			t.Run(`Success`, func(t *ftt.Test) {
				invocationID, testID, resultID, artifactID, err := ParseLegacyArtifactName("invocations/inv/tests/t/results/r/artifacts/a")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, invocationID, should.Equal("inv"))
				assert.Loosely(t, testID, should.Equal("t"))
				assert.Loosely(t, resultID, should.Equal("r"))
				assert.Loosely(t, artifactID, should.Equal("a"))
			})

			t.Run(`With a slash in test ID`, func(t *ftt.Test) {
				_, testID, _, _, err := ParseLegacyArtifactName("invocations/inv/tests/t%2F/results/r/artifacts/a/b")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, testID, should.Equal("t/"))
			})

			t.Run(`With a slash`, func(t *ftt.Test) {
				_, _, _, artifactID, err := ParseLegacyArtifactName("invocations/inv/tests/t/results/r/artifacts/a%2Fb")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, artifactID, should.Equal("a/b"))
			})
		})
	})
}

func TestLegacyArtifactName(t *testing.T) {
	t.Parallel()
	ftt.Run(`ArtifactName`, t, func(t *ftt.Test) {
		t.Run(`Invocation level`, func(t *ftt.Test) {
			t.Run(`Success`, func(t *ftt.Test) {
				name := LegacyInvocationArtifactName("inv", "a")
				assert.Loosely(t, name, should.Equal("invocations/inv/artifacts/a"))
			})
			t.Run(`With a slash`, func(t *ftt.Test) {
				name := LegacyInvocationArtifactName("inv", "a/b")
				assert.Loosely(t, name, should.Equal("invocations/inv/artifacts/a%2Fb"))
			})
		})

		t.Run(`Test result level`, func(t *ftt.Test) {
			t.Run(`Success`, func(t *ftt.Test) {
				name := LegacyTestResultArtifactName("inv", "t r", "r", "a")
				assert.Loosely(t, name, should.Equal("invocations/inv/tests/t%20r/results/r/artifacts/a"))
			})
			t.Run(`With a slash`, func(t *ftt.Test) {
				name := LegacyTestResultArtifactName("inv", "t r", "r", "a/b")
				assert.Loosely(t, name, should.Equal("invocations/inv/tests/t%20r/results/r/artifacts/a%2Fb"))
			})
		})
	})
}

func TestValidateLegacyArtifactName(t *testing.T) {
	t.Parallel()
	ftt.Run(`ValidateArtifactName`, t, func(t *ftt.Test) {
		t.Run(`Invocation level`, func(t *ftt.Test) {
			err := ValidateLegacyArtifactName("invocations/inv/artifacts/a/b")
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run(`Test result level`, func(t *ftt.Test) {
			err := ValidateLegacyArtifactName("invocations/inv/tests/t/results/r/artifacts/a")
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run(`Invalid`, func(t *ftt.Test) {
			err := ValidateLegacyArtifactName("abc")
			assert.Loosely(t, err, should.ErrLike("does not match"))
		})
	})
}

func TestArtifactId(t *testing.T) {
	t.Parallel()
	ftt.Run(`ValidateArtifactID`, t, func(t *ftt.Test) {
		t.Run(`ASCII printable`, func(t *ftt.Test) {
			err := ValidateArtifactID("ascii.txt")
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run(`Unicode printable`, func(t *ftt.Test) {
			err := ValidateArtifactID("unicode ©.txt")
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run(`Unprintable`, func(t *ftt.Test) {
			err := ValidateArtifactID("unprintable \a.txt")
			assert.Loosely(t, err, should.ErrLike("does not match"))
		})
		t.Run(`Starts with dot`, func(t *ftt.Test) {
			err := ValidateArtifactID(".arc.log")
			assert.Loosely(t, err, should.BeNil)
		})
	})
}

func TestIsTextArtifact(t *testing.T) {
	t.Parallel()
	ftt.Run("IsTextArtifact", t, func(t *ftt.Test) {
		t.Run("empty content type", func(t *ftt.Test) {
			assert.Loosely(t, IsTextArtifact(""), should.BeFalse)
		})
		t.Run("text artifact", func(t *ftt.Test) {
			assert.Loosely(t, IsTextArtifact("text/plain"), should.BeTrue)
		})
		t.Run("non text artifact", func(t *ftt.Test) {
			assert.Loosely(t, IsTextArtifact("image/png"), should.BeFalse)
		})
	})
}

func TestParseRbeURI(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		uri          string
		wantProject  string
		wantInstance string
		wantHash     string
		wantSize     int64
	}{
		{
			name:         "Valid URI",
			uri:          "bytestream://remotebuildexecution.googleapis.com/projects/my-proj/instances/default/blobs/e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855/12345",
			wantProject:  "my-proj",
			wantInstance: "default",
			wantHash:     "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			wantSize:     12345,
		},
		{
			name:         "Valid URI with zero size",
			uri:          "bytestream://us-central1-remotebuildexecution.googleapis.com/projects/another-proj/instances/test-inst/blobs/deadbeef/0",
			wantProject:  "another-proj",
			wantInstance: "test-inst",
			wantHash:     "deadbeef",
			wantSize:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			project, instance, hash, size, err := ParseRbeURI(tc.uri)

			if err != nil {
				t.Fatalf("ParseRbeURI(%q) failed unexpectedly: %v", tc.uri, err)
			}

			if project != tc.wantProject {
				t.Errorf("ParseRbeURI(%q) project = %q, want %q", tc.uri, project, tc.wantProject)
			}
			if instance != tc.wantInstance {
				t.Errorf("ParseRbeURI(%q) instance = %q, want %q", tc.uri, instance, tc.wantInstance)
			}
			if hash != tc.wantHash {
				t.Errorf("ParseRbeURI(%q) hash = %q, want %q", tc.uri, hash, tc.wantHash)
			}
			if size != tc.wantSize {
				t.Errorf("ParseRbeURI(%q) size = %d, want %d", tc.uri, size, tc.wantSize)
			}
		})
	}
}

func TestParseRbeURI_Failure(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		uri        string
		wantErrMsg string
	}{
		{
			name:       "Empty URI",
			uri:        "",
			wantErrMsg: "invalid RBE URI format",
		},
		{
			name:       "Not bytestream",
			uri:        "https://remotebuildexecution.googleapis.com/projects/my-proj/instances/default/blobs/abc/123",
			wantErrMsg: "invalid RBE URI format",
		},
		{
			name:       "Missing projects prefix",
			uri:        "bytestream://remotebuildexecution.googleapis.com/my-proj/instances/default/blobs/abc/123",
			wantErrMsg: "invalid RBE URI format",
		},
		{
			name:       "Missing instances prefix",
			uri:        "bytestream://remotebuildexecution.googleapis.com/projects/my-proj/default/blobs/abc/123",
			wantErrMsg: "invalid RBE URI format",
		},
		{
			name:       "Missing blobs prefix",
			uri:        "bytestream://remotebuildexecution.googleapis.com/projects/my-proj/instances/default/abc/123",
			wantErrMsg: "invalid RBE URI format",
		},
		{
			name:       "Unsupported resource type",
			uri:        "bytestream://remotebuildexecution.googleapis.com/projects/my-proj/instances/default/actions/abc",
			wantErrMsg: "invalid RBE URI format",
		},
		{
			name:       "Missing hash",
			uri:        "bytestream://remotebuildexecution.googleapis.com/projects/my-proj/instances/default/blobs//123",
			wantErrMsg: "invalid RBE URI format",
		},
		{
			name:       "Missing size",
			uri:        "bytestream://remotebuildexecution.googleapis.com/projects/my-proj/instances/default/blobs/abc/",
			wantErrMsg: "invalid RBE URI format",
		},
		{
			name:       "Non-numeric size",
			uri:        "bytestream://remotebuildexecution.googleapis.com/projects/my-proj/instances/default/blobs/abc/def",
			wantErrMsg: "invalid RBE URI format",
		},
		{
			name:       "Size not integer",
			uri:        "bytestream://remotebuildexecution.googleapis.com/projects/my-proj/instances/default/blobs/abc/123.45",
			wantErrMsg: "invalid RBE URI format",
		},
		{
			name:       "Empty project",
			uri:        "bytestream://remotebuildexecution.googleapis.com/projects//instances/default/blobs/abc/123",
			wantErrMsg: "invalid RBE URI format",
		},
		{
			name:       "Empty instance",
			uri:        "bytestream://remotebuildexecution.googleapis.com/projects/my-proj/instances//blobs/abc/123",
			wantErrMsg: "invalid RBE URI format",
		},
		{
			name:       "Size overflow",
			uri:        "bytestream://remotebuildexecution.googleapis.com/projects/my-proj/instances/default/blobs/abc/9223372036854775808", // MaxInt64 + 1
			wantErrMsg: "invalid size component in RBE URI",                                                                                 // Fails regex
		},
		{
			name:       "Too long",
			uri:        "bytestream://" + strings.Repeat("a", rbeURIMaxLength) + "/projects/my-proj/instances/default/blobs/abc/123",
			wantErrMsg: "too long; got 1086 bytes, want max 1024",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, _, err := ParseRbeURI(tc.uri)

			if err == nil {
				t.Fatalf("ParseRbeURI(%q) succeeded unexpectedly, want error", tc.uri)
			}
			if !strings.Contains(err.Error(), tc.wantErrMsg) {
				t.Errorf("ParseRbeURI(%q) returned error %v, want error containing %q", tc.uri, err, tc.wantErrMsg)
			}
		})
	}
}

func TestRbeInstancePath(t *testing.T) {
	t.Parallel()
	ftt.Run("RbeInstancePath", t, func(t *ftt.Test) {
		t.Run("Valid", func(t *ftt.Test) {
			path := RbeInstancePath("my-project", "my-instance")
			assert.Loosely(t, path, should.Equal("projects/my-project/instances/my-instance"))
		})
		t.Run("Empty project", func(t *ftt.Test) {
			path := RbeInstancePath("", "my-instance")
			assert.Loosely(t, path, should.Equal("projects//instances/my-instance"))
		})
		t.Run("Empty instance", func(t *ftt.Test) {
			path := RbeInstancePath("my-project", "")
			assert.Loosely(t, path, should.Equal("projects/my-project/instances/"))
		})
		t.Run("Empty both", func(t *ftt.Test) {
			path := RbeInstancePath("", "")
			assert.Loosely(t, path, should.Equal("projects//instances/"))
		})
	})
}
