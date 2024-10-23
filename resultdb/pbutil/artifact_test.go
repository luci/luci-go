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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"strings"
	"testing"
)

func TestParseArtifactName(t *testing.T) {
	t.Parallel()
	ftt.Run(`ParseArtifactName`, t, func(t *ftt.Test) {
		t.Run(`Invocation level`, func(t *ftt.Test) {
			t.Run(`Success`, func(t *ftt.Test) {
				invocationID, testID, resultID, artifactID, err := ParseArtifactName("invocations/inv/artifacts/a")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, invocationID, should.Equal("inv"))
				assert.Loosely(t, testID, should.BeEmpty)
				assert.Loosely(t, resultID, should.BeEmpty)
				assert.Loosely(t, artifactID, should.Equal("a"))
			})

			t.Run(`With a slash`, func(t *ftt.Test) {
				_, _, _, artifactID, err := ParseArtifactName("invocations/inv/artifacts/a%2Fb")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, artifactID, should.Equal("a/b"))
			})

			t.Run(`With a percent sign`, func(t *ftt.Test) {
				_, _, _, artifactID, err := ParseArtifactName("invocations/inv/artifacts/a%25b")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, artifactID, should.Equal("a%b"))
			})

			t.Run(`Success with a long artifact name`, func(t *ftt.Test) {
				artName := strings.Repeat("a%2Fb", 100) // 500 characters
				wantArtID := strings.Repeat("a/b", 100)
				_, _, _, gotArtID, err := ParseArtifactName("invocations/inv/artifacts/" + artName)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, gotArtID, should.Equal(wantArtID))
			})

			t.Run(`Failure with a long artifact name over the character limit`, func(t *ftt.Test) {
				artName := strings.Repeat("a", 600)
				_, _, _, _, err := ParseArtifactName("invocations/inv/artifacts/" + artName)
				assert.Loosely(t, err, should.NotBeNil)
			})
		})

		t.Run(`Test result level`, func(t *ftt.Test) {
			t.Run(`Success`, func(t *ftt.Test) {
				invocationID, testID, resultID, artifactID, err := ParseArtifactName("invocations/inv/tests/t/results/r/artifacts/a")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, invocationID, should.Equal("inv"))
				assert.Loosely(t, testID, should.Equal("t"))
				assert.Loosely(t, resultID, should.Equal("r"))
				assert.Loosely(t, artifactID, should.Equal("a"))
			})

			t.Run(`With a slash in test ID`, func(t *ftt.Test) {
				_, testID, _, _, err := ParseArtifactName("invocations/inv/tests/t%2F/results/r/artifacts/a/b")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, testID, should.Equal("t/"))
			})

			t.Run(`With a slash`, func(t *ftt.Test) {
				_, _, _, artifactID, err := ParseArtifactName("invocations/inv/tests/t/results/r/artifacts/a%2Fb")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, artifactID, should.Equal("a/b"))
			})
		})
	})
}

func TestArtifactName(t *testing.T) {
	t.Parallel()
	ftt.Run(`ArtifactName`, t, func(t *ftt.Test) {
		t.Run(`Invocation level`, func(t *ftt.Test) {
			t.Run(`Success`, func(t *ftt.Test) {
				name := InvocationArtifactName("inv", "a")
				assert.Loosely(t, name, should.Equal("invocations/inv/artifacts/a"))
			})
			t.Run(`With a slash`, func(t *ftt.Test) {
				name := InvocationArtifactName("inv", "a/b")
				assert.Loosely(t, name, should.Equal("invocations/inv/artifacts/a%2Fb"))
			})
		})

		t.Run(`Test result level`, func(t *ftt.Test) {
			t.Run(`Success`, func(t *ftt.Test) {
				name := TestResultArtifactName("inv", "t r", "r", "a")
				assert.Loosely(t, name, should.Equal("invocations/inv/tests/t%20r/results/r/artifacts/a"))
			})
			t.Run(`With a slash`, func(t *ftt.Test) {
				name := TestResultArtifactName("inv", "t r", "r", "a/b")
				assert.Loosely(t, name, should.Equal("invocations/inv/tests/t%20r/results/r/artifacts/a%2Fb"))
			})
		})
	})
}

func TestValidateArtifactName(t *testing.T) {
	t.Parallel()
	ftt.Run(`ValidateArtifactName`, t, func(t *ftt.Test) {
		t.Run(`Invocation level`, func(t *ftt.Test) {
			err := ValidateArtifactName("invocations/inv/artifacts/a/b")
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run(`Test result level`, func(t *ftt.Test) {
			err := ValidateArtifactName("invocations/inv/tests/t/results/r/artifacts/a")
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run(`Invalid`, func(t *ftt.Test) {
			err := ValidateArtifactName("abc")
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
