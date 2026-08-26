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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestFormatArtifactNames(t *testing.T) {
	t.Parallel()

	ftt.Run(`FormatTestResultArtifactName`, t, func(t *ftt.Test) {
		name := FormatTestResultArtifactName("build-123", "test:foo#bar", "0", "stdout")
		assert.Loosely(t, name, should.Equal("invocations/build-123/tests/test:foo%23bar/results/0/artifacts/stdout"))
	})

	ftt.Run(`FormatTestResultWorkUnitArtifactName`, t, func(t *ftt.Test) {
		name := FormatTestResultWorkUnitArtifactName("ants-123", "wu-456", "test:foo#bar", "0", "stdout")
		assert.Loosely(t, name, should.Equal("rootInvocations/ants-123/workUnits/wu-456/tests/test:foo%23bar/results/0/artifacts/stdout"))
	})

	ftt.Run(`FormatWorkUnitArtifactName`, t, func(t *ftt.Test) {
		name := FormatWorkUnitArtifactName("ants-123", "wu-456", "stderr")
		assert.Loosely(t, name, should.Equal("rootInvocations/ants-123/workUnits/wu-456/artifacts/stderr"))
	})
}

func TestValidateArtifactFlags(t *testing.T) {
	t.Parallel()

	ftt.Run(`ValidateArtifactFlags`, t, func(t *ftt.Test) {
		t.Run(`missing artifact ID`, func(t *ftt.Test) {
			err := ValidateArtifactFlags(ParentTypeTestResult, "inv", "", "test", "0", "")
			assert.Loosely(t, err, should.ErrLike("flag -artifactid is required"))
		})

		t.Run(`test result parent missing required flags`, func(t *ftt.Test) {
			err := ValidateArtifactFlags(ParentTypeTestResult, "", "", "test", "0", "stdout")
			assert.Loosely(t, err, should.ErrLike("flags -invocationid, -testid, -resultid, and -artifactid are required"))

			err = ValidateArtifactFlags(ParentTypeTestResult, "inv", "", "", "0", "stdout")
			assert.Loosely(t, err, should.ErrLike("flags -invocationid, -testid, -resultid, and -artifactid are required"))

			err = ValidateArtifactFlags(ParentTypeTestResult, "inv", "", "test", "", "stdout")
			assert.Loosely(t, err, should.ErrLike("flags -invocationid, -testid, -resultid, and -artifactid are required"))
		})

		t.Run(`test result parent valid`, func(t *ftt.Test) {
			err := ValidateArtifactFlags(ParentTypeTestResult, "inv", "", "test", "0", "stdout")
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`work unit parent missing required flags`, func(t *ftt.Test) {
			err := ValidateArtifactFlags(ParentTypeWorkUnit, "", "wu-1", "", "", "stdout")
			assert.Loosely(t, err, should.ErrLike("flags -invocationid, -workunitid, and -artifactid are required"))

			err = ValidateArtifactFlags(ParentTypeWorkUnit, "inv", "", "", "", "stdout")
			assert.Loosely(t, err, should.ErrLike("flags -invocationid, -workunitid, and -artifactid are required"))
		})

		t.Run(`work unit parent valid`, func(t *ftt.Test) {
			err := ValidateArtifactFlags(ParentTypeWorkUnit, "inv", "wu-1", "", "", "stdout")
			assert.Loosely(t, err, should.BeNil)
		})
	})
}

func TestResolveResourceAndArtifactNames(t *testing.T) {
	t.Parallel()

	ftt.Run(`ResolveTestResultResourceName`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`legacy mode`, func(t *ftt.Test) {
			res, err := ResolveTestResultResourceName(ctx, nil, "build-123", "", "test1", "res1", true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("invocations/build-123/tests/test1/results/res1"))
		})

		t.Run(`explicit work unit`, func(t *ftt.Test) {
			res, err := ResolveTestResultResourceName(ctx, nil, "ants-i123", "wu-999", "test1", "res1", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("rootInvocations/ants-i123/workUnits/wu-999/tests/test1/results/res1"))
		})

		t.Run(`query verdict fallback`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					return &pb.QueryTestVerdictsResponse{
						TestVerdicts: []*pb.TestVerdict{
							{
								TestId: "test1",
								Results: []*pb.TestResult{
									{
										Name:     "rootInvocations/ants-i123/workUnits/discovered-wu/tests/test1/results/res1",
										TestId:   "test1",
										ResultId: "res1",
									},
								},
							},
						},
					}, nil
				},
			}
			res, err := ResolveTestResultResourceName(ctx, client, "ants-i123", "", "test1", "res1", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("rootInvocations/ants-i123/workUnits/discovered-wu/tests/test1/results/res1"))

			artName, err := ResolveArtifactResourceName(ctx, client, ParentTypeTestResult, "ants-i123", "", "test1", "res1", "snippet", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, artName, should.Equal("rootInvocations/ants-i123/workUnits/discovered-wu/tests/test1/results/res1/artifacts/snippet"))
		})

		t.Run(`not found`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				queryTestVerdicts: func(ctx context.Context, in *pb.QueryTestVerdictsRequest) (*pb.QueryTestVerdictsResponse, error) {
					return &pb.QueryTestVerdictsResponse{}, nil
				},
			}
			_, err := ResolveTestResultResourceName(ctx, client, "ants-i123", "", "test1", "nonexistent", false)
			assert.Loosely(t, err, should.ErrLike("not found"))
		})
		t.Run(`ResolveTargetResourceName work unit`, func(t *ftt.Test) {
			res, err := ResolveTargetResourceName(ctx, nil, ParentTypeWorkUnit, "build-123", "wu-1", "", "", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("rootInvocations/build-123/workUnits/wu-1"))

			artName, err := ResolveArtifactResourceName(ctx, nil, ParentTypeWorkUnit, "build-123", "wu-1", "", "", "syslog", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, artName, should.Equal("rootInvocations/build-123/workUnits/wu-1/artifacts/syslog"))
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
