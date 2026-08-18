// Copyright 2026 The LUCI Authors.
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

package artifact

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestValidateTargetParent(t *testing.T) {
	t.Parallel()

	ftt.Run(`ValidateTargetParent`, t, func(t *ftt.Test) {
		t.Run(`test result under test result command`, func(t *ftt.Test) {
			err := ValidateTargetParent("invocations/build-123/tests/test1/results/0", ParentTypeTestResult)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`work unit under test result command`, func(t *ftt.Test) {
			err := ValidateTargetParent("rootInvocations/build-123/workUnits/wu1", ParentTypeTestResult)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("please use 'luci work-unit artifact ...'"))
		})

		t.Run(`test result under work unit command`, func(t *ftt.Test) {
			err := ValidateTargetParent("invocations/build-123/tests/test1/results/0", ParentTypeWorkUnit)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("please use 'luci test-result artifact ...'"))
		})
	})
}

func TestResolveArtifactName(t *testing.T) {
	t.Parallel()

	ftt.Run(`ResolveArtifactName`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`direct work unit artifact`, func(t *ftt.Test) {
			name := "rootInvocations/root1/workUnits/wu1/artifacts/stdout"
			res, err := ResolveArtifactName(ctx, nil, name, ParentTypeWorkUnit, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal(name))
		})

		t.Run(`direct test result artifact with full URL`, func(t *ftt.Test) {
			url := "https://ci.chromium.org/ui/invocations/build-123/tests/test1/results/0/artifacts/screenshot.png"
			res, err := ResolveArtifactName(ctx, nil, url, ParentTypeTestResult, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("invocations/build-123/tests/test1/results/0/artifacts/screenshot.png"))
		})

		t.Run(`test result URL with query parameter`, func(t *ftt.Test) {
			url := "https://ci.chromium.org/ui/invocations/build-123/tests/test1/results/0?artifact=stdout"
			res, err := ResolveArtifactName(ctx, nil, url, ParentTypeTestResult, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("invocations/build-123/tests/test1/results/0/artifacts/stdout"))
		})

		t.Run(`invalid artifact name`, func(t *ftt.Test) {
			_, err := ResolveArtifactName(ctx, nil, "invocations/build-123/tests/test1", ParentTypeTestResult, false)
			assert.Loosely(t, err, should.NotBeNil)
		})
	})
}

func TestParseTargetArgs(t *testing.T) {
	ftt.Run(`ParseTargetArgs with single '-' and trailing overrides`, t, func(t *ftt.Test) {
		tempDir := t.TempDir()
		base.SetTestCacheDir(tempDir)

		t.Run(`test result target args`, func(t *ftt.Test) {
			base.RecordTestResult("invocations/build-100/tests/my_test/results/0", "build-100", "my_test")

			// - (reuse full test result)
			res, err := base.ParseTestResultTargetArgs([]string{"-"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("invocations/build-100/tests/my_test/results/0"))

			// - <result_id> (override result_id)
			res, err = base.ParseTestResultTargetArgs([]string{"-", "1"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("invocations/build-100/tests/my_test/results/1"))

			// - <test_id> <result_id> (override test_id and result_id)
			res, err = base.ParseTestResultTargetArgs([]string{"-", "other_test", "2"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("invocations/build-100/tests/other_test/results/2"))

			// single short name without '-' must error
			_, err = base.ParseTestResultTargetArgs([]string{"1"})
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("use '-' to reference cached context"))
		})

		t.Run(`work unit target args`, func(t *ftt.Test) {
			base.RecordWorkUnit("rootInvocations/build-100/workUnits/wu1", "build-100")

			// - (reuse full work unit)
			res, err := base.ParseWorkUnitTargetArgs([]string{"-"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("rootInvocations/build-100/workUnits/wu1"))

			// - <work_unit_id> (override work_unit_id)
			res, err = base.ParseWorkUnitTargetArgs([]string{"-", "wu2"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("rootInvocations/build-100/workUnits/wu2"))

			// single short name without '-' must error
			_, err = base.ParseWorkUnitTargetArgs([]string{"wu2"})
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("use '-' to reference cached context"))
		})
	})
}

func TestResolveArtifactTarget(t *testing.T) {
	ftt.Run(`ResolveArtifactTarget`, t, func(t *ftt.Test) {
		ctx := context.Background()
		tempDir := t.TempDir()
		base.SetTestCacheDir(tempDir)

		t.Run(`single full artifact name`, func(t *ftt.Test) {
			name := "invocations/build-123/tests/test1/results/0/artifacts/stdout"
			res, err := ResolveArtifactTarget(ctx, nil, []string{name}, ParentTypeTestResult, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal(name))
		})

		t.Run(`target and artifact ID args records artifact for future '-' calls`, func(t *ftt.Test) {
			target := "invocations/build-123/tests/test1/results/0"
			// 1. target and artifact ID
			res, err := ResolveArtifactTarget(ctx, nil, []string{target, "snippet"}, ParentTypeTestResult, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("invocations/build-123/tests/test1/results/0/artifacts/snippet"))

			// 2. single '-' reuses both test result and artifact
			res, err = ResolveArtifactTarget(ctx, nil, []string{"-"}, ParentTypeTestResult, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("invocations/build-123/tests/test1/results/0/artifacts/snippet"))

			// 3. '-' with override artifact ID
			res, err = ResolveArtifactTarget(ctx, nil, []string{"-", "other_art"}, ParentTypeTestResult, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("invocations/build-123/tests/test1/results/0/artifacts/other_art"))

			// 4. single '-' reuses new artifact
			res, err = ResolveArtifactTarget(ctx, nil, []string{"-"}, ParentTypeTestResult, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("invocations/build-123/tests/test1/results/0/artifacts/other_art"))
		})

		t.Run(`single '-' with 1 trailing override for test result artifact`, func(t *ftt.Test) {
			base.RecordTestResult("invocations/build-456/tests/test2/results/1", "build-456", "test2")

			// - <artifact_id>: reuse full test result, supply artifact_id
			res, err := ResolveArtifactTarget(ctx, nil, []string{"-", "stderr"}, ParentTypeTestResult, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("invocations/build-456/tests/test2/results/1/artifacts/stderr"))
		})

		t.Run(`single '-' with 2 trailing overrides for test result artifact`, func(t *ftt.Test) {
			base.RecordTestResult("invocations/build-456/tests/test2/results/1", "build-456", "test2")

			// - <result_id> <artifact_id>: reuse invocation & test_id, supply result_id and artifact_id
			res, err := ResolveArtifactTarget(ctx, nil, []string{"-", "3", "stdout"}, ParentTypeTestResult, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("invocations/build-456/tests/test2/results/3/artifacts/stdout"))
		})

		t.Run(`single '-' with 3 trailing overrides for test result artifact`, func(t *ftt.Test) {
			base.RecordTestResult("invocations/build-456/tests/test2/results/1", "build-456", "test2")

			// - <test_id> <result_id> <artifact_id>: reuse invocation, supply test_id, result_id, artifact_id
			res, err := ResolveArtifactTarget(ctx, nil, []string{"-", "new_test", "0", "trace"}, ParentTypeTestResult, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("invocations/build-456/tests/new_test/results/0/artifacts/trace"))
		})

		t.Run(`work unit and artifact ID args`, func(t *ftt.Test) {
			target := "rootInvocations/root1/workUnits/wu1"
			res, err := ResolveArtifactTarget(ctx, nil, []string{target, "stderr"}, ParentTypeWorkUnit, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("rootInvocations/root1/workUnits/wu1/artifacts/stderr"))
		})

		t.Run(`single '-' with 1 trailing override for work unit artifact`, func(t *ftt.Test) {
			base.RecordWorkUnit("rootInvocations/build-500/workUnits/run-tests", "build-500")

			// - <artifact_id>: reuse work unit, supply artifact_id
			res, err := ResolveArtifactTarget(ctx, nil, []string{"-", "stdout"}, ParentTypeWorkUnit, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("rootInvocations/build-500/workUnits/run-tests/artifacts/stdout"))
		})

		t.Run(`single '-' with 2 trailing overrides for work unit artifact`, func(t *ftt.Test) {
			base.RecordWorkUnit("rootInvocations/build-500/workUnits/run-tests", "build-500")

			// - <work_unit_id> <artifact_id>: reuse root invocation, supply work_unit_id and artifact_id
			res, err := ResolveArtifactTarget(ctx, nil, []string{"-", "other-wu", "custom_log"}, ParentTypeWorkUnit, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("rootInvocations/build-500/workUnits/other-wu/artifacts/custom_log"))
		})

		t.Run(`decomposed 3 args for work unit`, func(t *ftt.Test) {
			res, err := ResolveArtifactTarget(ctx, nil, []string{"build-123", "run-tests", "stdout"}, ParentTypeWorkUnit, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("rootInvocations/build-123/workUnits/run-tests/artifacts/stdout"))
		})

		t.Run(`mismatched work unit under test result`, func(t *ftt.Test) {
			_, err := ResolveArtifactTarget(ctx, nil, []string{"rootInvocations/root1/workUnits/wu1", "stderr"}, ParentTypeTestResult, false)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("please use 'luci work-unit artifact ...'"))
		})

		t.Run(`user scenario: work-unit then test-result with new invocation invalidates work unit`, func(t *ftt.Test) {
			// 1. work-unit artifact get on inv1: ok
			base.RecordWorkUnit("rootInvocations/build-100/workUnits/wu1", "build-100")
			res, err := ResolveArtifactTarget(ctx, nil, []string{"-", "artifactid"}, ParentTypeWorkUnit, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("rootInvocations/build-100/workUnits/wu1/artifacts/artifactid"))

			// 2. test-result get on inv2: clears work unit
			base.RecordTestResult("invocations/build-200/tests/testid/results/resultid", "build-200", "testid")

			// 3. work-unit artifact get - artifactid: fails asking to specify work unit
			_, err = ResolveArtifactTarget(ctx, nil, []string{"-", "artifactid"}, ParentTypeWorkUnit, false)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("no previous work unit found in cache"))
		})

		t.Run(`missing args`, func(t *ftt.Test) {
			_, err := ResolveArtifactTarget(ctx, nil, nil, ParentTypeTestResult, false)
			assert.Loosely(t, err, should.NotBeNil)
		})
	})
}

func TestFetchHTTPByteRangeEmptyFile(t *testing.T) {
	t.Parallel()

	ftt.Run(`FetchHTTPByteRange on empty file`, t, func(t *ftt.Test) {
		ctx := context.Background()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			serveHTTPRange(w, r, []byte(""))
		}))
		defer server.Close()

		var buf bytes.Buffer
		byteRange := &ByteRange{Start: -1, End: 65536}
		code, size, err := FetchHTTPByteRange(ctx, server.Client(), server.URL, byteRange, &buf)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, code, should.Equal(http.StatusRequestedRangeNotSatisfiable))
		assert.Loosely(t, size, should.Equal(0))
		assert.Loosely(t, buf.Len(), should.Equal(0))
	})
}

// serveHTTPRange is a mock HTTP handler that emulates standard HTTP Range requests.
func serveHTTPRange(w http.ResponseWriter, r *http.Request, content []byte) {
	rangeHdr := r.Header.Get("Range")
	total := int64(len(content))
	if rangeHdr == "" {
		w.WriteHeader(http.StatusOK)
		w.Write(content)
		return
	}

	br, err := ParseByteRange(strings.TrimPrefix(rangeHdr, "bytes="))
	if err != nil || br == nil {
		w.WriteHeader(http.StatusOK)
		w.Write(content)
		return
	}

	var start, end int64
	if br.Start < 0 && br.End >= 0 {
		// Suffix range: bytes=-N
		if br.End >= total {
			start = 0
			end = total - 1
		} else {
			start = total - br.End
			end = total - 1
		}
	} else if br.Start >= 0 && br.End < 0 {
		// bytes=S-
		start = br.Start
		end = total - 1
	} else {
		// bytes=S-E
		start = br.Start
		end = br.End
		if end >= total {
			end = total - 1
		}
	}

	if start < 0 {
		start = 0
	}
	if start > end || start >= total {
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}

	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, total))
	w.WriteHeader(http.StatusPartialContent)
	w.Write(content[start : end+1])
}
