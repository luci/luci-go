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
	"errors"
	"fmt"
	"io"
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

	ftt.Run(`FormatInvocationArtifactName`, t, func(t *ftt.Test) {
		name := FormatInvocationArtifactName("build-123", "minidump", true)
		assert.Loosely(t, name, should.Equal("invocations/build-123/artifacts/minidump"))

		nameRoot := FormatInvocationArtifactName("build-123", "minidump", false)
		assert.Loosely(t, nameRoot, should.Equal("rootInvocations/build-123/artifacts/minidump"))

		nameSpecial := FormatInvocationArtifactName("build-123", "tradefed/pkg.Class#Method [0]?foo=bar.png", true)
		assert.Loosely(t, nameSpecial, should.Equal("invocations/build-123/artifacts/tradefed%2Fpkg.Class%23Method%20%5B0%5D%3Ffoo=bar.png"))
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

		t.Run(`invocation parent missing required flags`, func(t *ftt.Test) {
			err := ValidateArtifactFlags(ParentTypeInvocation, "", "", "", "", "stdout")
			assert.Loosely(t, err, should.ErrLike("flags -invocationid and -artifactid are required"))
		})

		t.Run(`invocation parent valid`, func(t *ftt.Test) {
			err := ValidateArtifactFlags(ParentTypeInvocation, "inv", "", "", "", "stdout")
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`unknown parent missing required flags`, func(t *ftt.Test) {
			err := ValidateArtifactFlags(ParentTypeUnknown, "", "", "", "", "stdout")
			assert.Loosely(t, err, should.ErrLike("flags -invocationid, -artifactid, and either -workunitid (for work unit artifacts) or -testid and -resultid (for test result artifacts) are required"))

			err = ValidateArtifactFlags(ParentTypeUnknown, "", "", "test", "", "stdout")
			assert.Loosely(t, err, should.ErrLike("flags -invocationid, -testid, -resultid, and -artifactid are required for test result artifacts"))

			err = ValidateArtifactFlags(ParentTypeUnknown, "", "", "test", "0", "stdout")
			assert.Loosely(t, err, should.ErrLike("flags -invocationid, -testid, -resultid, and -artifactid are required for test result artifacts"))

			err = ValidateArtifactFlags(ParentTypeUnknown, "", "wu-1", "", "", "stdout")
			assert.Loosely(t, err, should.ErrLike("flags -invocationid, -workunitid, and -artifactid are required"))
		})

		t.Run(`unknown parent valid`, func(t *ftt.Test) {
			err := ValidateArtifactFlags(ParentTypeUnknown, "inv", "", "test", "0", "stdout")
			assert.Loosely(t, err, should.BeNil)

			err = ValidateArtifactFlags(ParentTypeUnknown, "inv", "wu-1", "", "", "stdout")
			assert.Loosely(t, err, should.BeNil)

			err = ValidateArtifactFlags(ParentTypeUnknown, "inv", "", "", "", "stdout")
			assert.Loosely(t, err, should.BeNil)
		})
	})
}

func TestResolveParentType(t *testing.T) {
	t.Parallel()

	ftt.Run(`ResolveParentType`, t, func(t *ftt.Test) {
		t.Run(`test result valid`, func(t *ftt.Test) {
			pt, err := ResolveParentType("inv", "", "test", "0")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pt, should.Equal(ParentTypeTestResult))

			pt, err = ResolveParentType("inv", "wu-1", "test", "0")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pt, should.Equal(ParentTypeTestResult))
		})

		t.Run(`test result missing flags`, func(t *ftt.Test) {
			_, err := ResolveParentType("inv", "", "test", "")
			assert.Loosely(t, err, should.ErrLike("flags -invocationid, -testid, and -resultid are required for test result artifacts"))

			_, err = ResolveParentType("", "", "test", "0")
			assert.Loosely(t, err, should.ErrLike("flags -invocationid, -testid, and -resultid are required for test result artifacts"))
		})

		t.Run(`work unit valid`, func(t *ftt.Test) {
			pt, err := ResolveParentType("inv", "wu-1", "", "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pt, should.Equal(ParentTypeWorkUnit))
		})

		t.Run(`work unit missing invocation ID`, func(t *ftt.Test) {
			_, err := ResolveParentType("", "wu-1", "", "")
			assert.Loosely(t, err, should.ErrLike("flags -invocationid and -workunitid are required"))
		})

		t.Run(`invocation valid`, func(t *ftt.Test) {
			pt, err := ResolveParentType("inv", "", "", "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pt, should.Equal(ParentTypeInvocation))
		})

		t.Run(`neither specified`, func(t *ftt.Test) {
			_, err := ResolveParentType("", "", "", "")
			assert.Loosely(t, err, should.ErrLike("flags -invocationid and either -workunitid (for work unit artifacts) or -testid and -resultid (for test result artifacts) are required"))
		})
	})
}

func TestResolveResourceAndArtifactNames(t *testing.T) {
	t.Parallel()

	ftt.Run(`ResolveTestResultResourceName`, t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run(`legacy mode queries test result`, func(t *ftt.Test) {
			client := &mockResultDBClient{
				queryTestResults: func(ctx context.Context, in *pb.QueryTestResultsRequest) (*pb.QueryTestResultsResponse, error) {
					return &pb.QueryTestResultsResponse{
						TestResults: []*pb.TestResult{
							{
								Name:     "invocations/u-leaf/tests/test1/results/res1",
								TestId:   "test1",
								ResultId: "res1",
							},
						},
					}, nil
				},
			}
			res, err := ResolveTestResultResourceName(ctx, client, "build-123", "", "test1", "res1", true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("invocations/u-leaf/tests/test1/results/res1"))
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

		t.Run(`ResolveTargetResourceName invocation`, func(t *ftt.Test) {
			res, err := ResolveTargetResourceName(ctx, nil, ParentTypeInvocation, "build-123", "", "", "", true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("invocations/build-123"))

			artName, err := ResolveArtifactResourceName(ctx, nil, ParentTypeInvocation, "build-123", "", "", "", "minidump", true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, artName, should.Equal("invocations/build-123/artifacts/minidump"))
		})

		t.Run(`ResolveTargetResourceName invocation unknown parent`, func(t *ftt.Test) {
			res, err := ResolveTargetResourceName(ctx, nil, ParentTypeUnknown, "build-123", "", "", "", true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("invocations/build-123"))

			artName, err := ResolveArtifactResourceName(ctx, nil, ParentTypeUnknown, "build-123", "", "", "", "minidump", true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, artName, should.Equal("invocations/build-123/artifacts/minidump"))
		})

		t.Run(`ResolveTargetResourceName with ParentTypeUnknown`, func(t *ftt.Test) {
			res, err := ResolveTargetResourceName(ctx, nil, ParentTypeUnknown, "build-123", "wu-1", "", "", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("rootInvocations/build-123/workUnits/wu-1"))

			res, err = ResolveTargetResourceName(ctx, nil, ParentTypeUnknown, "build-123", "wu-1", "test1", "res1", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("rootInvocations/build-123/workUnits/wu-1/tests/test1/results/res1"))
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

// chunkedReader yields a predetermined sequence of byte chunks on successive Read calls
// to deterministically test ring-buffer state transitions in copySuffix.
type chunkedReader struct {
	chunks [][]byte
	idx    int
	err    error
}

func (r *chunkedReader) Read(p []byte) (int, error) {
	if r.idx >= len(r.chunks) {
		if r.err != nil {
			return 0, r.err
		}
		return 0, io.EOF
	}
	n := copy(p, r.chunks[r.idx])
	r.idx++
	return n, nil
}

// failAfterWriter succeeds for the first allowedCalls Write calls and returns writeErr thereafter.
type failAfterWriter struct {
	allowedCalls int
	calls        int
	writeErr     error
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	if w.calls >= w.allowedCalls {
		return 0, w.writeErr
	}
	w.calls++
	return len(p), nil
}

func TestCopySuffix(t *testing.T) {
	t.Parallel()

	ftt.Run(`copySuffix`, t, func(t *ftt.Test) {
		t.Run(`suffixLen <= 0 discards stream`, func(t *ftt.Test) {
			for _, suffixLen := range []int64{0, -5} {
				var buf bytes.Buffer
				nRead, err := copySuffix(&buf, strings.NewReader("abcdefghij"), suffixLen)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, nRead, should.Equal(int64(10)))
				assert.Loosely(t, buf.Len(), should.Equal(0))
			}
		})

		t.Run(`empty stream`, func(t *ftt.Test) {
			var buf bytes.Buffer
			nRead, err := copySuffix(&buf, strings.NewReader(""), 10)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nRead, should.Equal(int64(0)))
			assert.Loosely(t, buf.Len(), should.Equal(0))
		})

		t.Run(`single chunk >= suffixLen`, func(t *ftt.Test) {
			// Exact match (len(chunk) == suffixLen)
			var buf bytes.Buffer
			nRead, err := copySuffix(&buf, &chunkedReader{chunks: [][]byte{[]byte("abcdef")}}, 6)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nRead, should.Equal(int64(6)))
			assert.Loosely(t, buf.String(), should.Equal("abcdef"))

			// Strictly larger (len(chunk) > suffixLen)
			buf.Reset()
			nRead, err = copySuffix(&buf, &chunkedReader{chunks: [][]byte{[]byte("abcdefghij")}}, 4)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nRead, should.Equal(int64(10)))
			assert.Loosely(t, buf.String(), should.Equal("ghij"))
		})

		t.Run(`small chunks followed by large chunk >= suffixLen and trailing small chunk`, func(t *ftt.Test) {
			// Exercises: !wrapped accumulation -> chunk >= suffixLen superseding ring -> wrapped overwrite.
			r := &chunkedReader{
				chunks: [][]byte{
					[]byte("ab"),       // !wrapped: ring = "ab"
					[]byte("01234567"), // >= suffixLen (5): ring = "34567", writePos = 0, wrapped = true
					[]byte("XY"),       // wrapped overwrite at writePos 0: ring = "XY567", writePos = 2
				},
			}
			var buf bytes.Buffer
			nRead, err := copySuffix(&buf, r, 5)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nRead, should.Equal(int64(12)))
			assert.Loosely(t, buf.String(), should.Equal("567XY"))
		})

		t.Run(`multiple small chunks never reaching suffixLen (!wrapped at EOF)`, func(t *ftt.Test) {
			r := &chunkedReader{
				chunks: [][]byte{
					[]byte("ab"),
					[]byte("cd"),
					[]byte("ef"),
				},
			}
			var buf bytes.Buffer
			nRead, err := copySuffix(&buf, r, 10)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nRead, should.Equal(int64(6)))
			assert.Loosely(t, buf.String(), should.Equal("abcdef"))
		})

		t.Run(`multiple small chunks hitting suffixLen exactly`, func(t *ftt.Test) {
			// Exercises int64(len(ring)) == suffixLen transition inside !wrapped.
			r := &chunkedReader{
				chunks: [][]byte{
					[]byte("ab"),
					[]byte("cde"), // len(ring) becomes 5 == suffixLen -> wrapped = true, writePos = 0
				},
			}
			var buf bytes.Buffer
			nRead, err := copySuffix(&buf, r, 5)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nRead, should.Equal(int64(5)))
			assert.Loosely(t, buf.String(), should.Equal("abcde"))

			// Now with an additional chunk after hitting exact suffixLen.
			r2 := &chunkedReader{
				chunks: [][]byte{
					[]byte("ab"),
					[]byte("cde"), // wrapped = true, writePos = 0
					[]byte("fg"),  // overwrites "ab" -> "fgcde", writePos = 2
				},
			}
			buf.Reset()
			nRead, err = copySuffix(&buf, r2, 5)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nRead, should.Equal(int64(7)))
			assert.Loosely(t, buf.String(), should.Equal("cdefg"))
		})

		t.Run(`small chunks crossing suffixLen threshold and wrapping around multiple times`, func(t *ftt.Test) {
			// Exercises:
			// 1. !wrapped growing slice ("abc", len 3 < 5)
			// 2. !wrapped crossing suffixLen ("defg", 3+4 > 5) -> allocates fullRing "cdefg", writePos = 0, wrapped = true
			// 3. wrapped where len(chunk) <= firstPart ("hi", len 2 <= 5) -> ring = "hidefg"[0:5]="hideg", writePos = 2
			// 4. wrapped where len(chunk) > firstPart ("JKLM", len 4 > 5-2=3) -> splits across end ("JKL" at [2:5]) and start ("M" at [0:1]), writePos = 1
			// 5. wrapped where writePos reaches end of ring exactly ("NO", len 2, writePos 1->3; then "PQ", len 2 == 5-3, writePos wraps to 0)
			r := &chunkedReader{
				chunks: [][]byte{
					[]byte("abc"),
					[]byte("defg"),
					[]byte("hi"),
					[]byte("JKLM"),
				},
			}
			var buf bytes.Buffer
			nRead, err := copySuffix(&buf, r, 5)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nRead, should.Equal(int64(13)))
			assert.Loosely(t, buf.String(), should.Equal("iJKLM"))

			// Continue wrapping back to writePos == 0.
			rExactWrap := &chunkedReader{
				chunks: [][]byte{
					[]byte("abc"),
					[]byte("defg"),
					[]byte("hi"),
					[]byte("JKLM"), // writePos = 1
					[]byte("NO"),   // writePos = 3
					[]byte("PQ"),   // writePos = (3+2)%5 = 0
				},
			}
			buf.Reset()
			nRead, err = copySuffix(&buf, rExactWrap, 5)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nRead, should.Equal(int64(17)))
			assert.Loosely(t, buf.String(), should.Equal("MNOPQ"))
		})

		t.Run(`exhaustive sweep over chunk sizes and suffix lengths`, func(t *ftt.Test) {
			payload := make([]byte, 120)
			for i := range payload {
				payload[i] = byte('!' + (i % 90))
			}

			for _, chunkSize := range []int{1, 2, 3, 7, 13, 32, 64, 150} {
				var chunks [][]byte
				for off := 0; off < len(payload); off += chunkSize {
					end := off + chunkSize
					if end > len(payload) {
						end = len(payload)
					}
					chunks = append(chunks, payload[off:end])
				}

				for _, suffixLen := range []int64{1, 2, 5, 7, 13, 31, 32, 33, 64, 119, 120, 121, 200} {
					var buf bytes.Buffer
					r := &chunkedReader{chunks: chunks}
					nRead, err := copySuffix(&buf, r, suffixLen)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, nRead, should.Equal(int64(len(payload))))

					wantStart := int64(len(payload)) - suffixLen
					if wantStart < 0 {
						wantStart = 0
					}
					assert.Loosely(t, buf.Bytes(), should.Resemble(payload[wantStart:]))
				}
			}
		})

		t.Run(`error propagation from src and dst`, func(t *ftt.Test) {
			readErr := errors.New("simulated read failure")
			r := &chunkedReader{
				chunks: [][]byte{[]byte("abc"), []byte("def")},
				err:    readErr,
			}
			var buf bytes.Buffer
			nRead, err := copySuffix(&buf, r, 4)
			assert.Loosely(t, err, should.Equal(readErr))
			assert.Loosely(t, nRead, should.Equal(int64(6)))

			writeErr := errors.New("simulated write failure")
			// Fail on first Write when !wrapped
			_, err = copySuffix(&failAfterWriter{allowedCalls: 0, writeErr: writeErr}, &chunkedReader{chunks: [][]byte{[]byte("abc")}}, 10)
			assert.Loosely(t, err, should.Equal(writeErr))

			// Fail on first Write (ring[writePos:]) when wrapped
			_, err = copySuffix(&failAfterWriter{allowedCalls: 0, writeErr: writeErr}, &chunkedReader{chunks: [][]byte{[]byte("abcde"), []byte("fg")}}, 5)
			assert.Loosely(t, err, should.Equal(writeErr))

			// Fail on second Write (ring[:writePos]) when wrapped with writePos > 0
			_, err = copySuffix(&failAfterWriter{allowedCalls: 1, writeErr: writeErr}, &chunkedReader{chunks: [][]byte{[]byte("abcde"), []byte("fg")}}, 5)
			assert.Loosely(t, err, should.Equal(writeErr))
		})
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
