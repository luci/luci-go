// Copyright 2025 The LUCI Authors.
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

package artifacts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestProcessComparisonReader(t *testing.T) {
	t.Parallel()

	ftt.Run("ProcessComparisonReader", t, func(t *ftt.Test) {
		t.Run("with simple reader", func(t *ftt.Test) {
			reader := bytes.NewReader([]byte("line 1\nline 2\nline 3\nline 4"))
			hashes, err := ProcessComparisonReader(context.Background(), reader)
			assert.Loosely(t, err, should.BeNil)

			expectedHashes := make(map[int64]struct{})
			expectedHashes[mustHash("line 1")] = struct{}{}
			expectedHashes[mustHash("line 2")] = struct{}{}
			expectedHashes[mustHash("line 3")] = struct{}{}
			expectedHashes[mustHash("line 4")] = struct{}{}
			assert.Loosely(t, hashes, should.Resemble(expectedHashes))
		})

		t.Run("with line split across reads", func(t *ftt.Test) {
			reader := bytes.NewReader([]byte("line 1\nline 2\nline 3\nline 4"))
			hashes, err := ProcessComparisonReader(context.Background(), reader)
			assert.Loosely(t, err, should.BeNil)

			expectedHashes := make(map[int64]struct{})
			expectedHashes[mustHash("line 1")] = struct{}{}
			expectedHashes[mustHash("line 2")] = struct{}{}
			expectedHashes[mustHash("line 3")] = struct{}{}
			expectedHashes[mustHash("line 4")] = struct{}{}
			assert.Loosely(t, hashes, should.Resemble(expectedHashes))
		})

		t.Run("detects long line", func(t *ftt.Test) {
			reader := bytes.NewReader(bytes.Repeat([]byte("a"), maxLineLengthBytes+1))
			_, err := ProcessComparisonReader(context.Background(), reader)
			assert.Loosely(t, err, should.Equal(errBinaryFileDetected))
		})

		t.Run("detects invalid utf8", func(t *ftt.Test) {
			reader := bytes.NewReader(append([]byte("valid line\n"), 0xff, 0xfe, 0xfd))
			_, err := ProcessComparisonReader(context.Background(), reader)
			assert.Loosely(t, err, should.Equal(errBinaryFileDetected))
		})
	})
}

func TestProcessFailingReader(t *testing.T) {
	t.Parallel()

	passingContent := `line a
line c
line e`
	passingHashes := make(map[int64]struct{})
	for _, line := range bytes.Split([]byte(passingContent), []byte("\n")) {
		h, _ := hashLine(line)
		passingHashes[h] = struct{}{}
	}

	failingContent := `line a
line b
line c
line d1
line d2
line e
line f`

	ftt.Run("ProcessFailingReader", t, func(t *ftt.Test) {
		t.Run("identifies failure ranges correctly, ranges only", func(t *ftt.Test) {
			reader := strings.NewReader(failingContent)
			resp, err := ProcessFailingReader(context.Background(), reader, passingHashes, pb.CompareArtifactLinesRequest_RANGES_ONLY, 1000, 0, 0)

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.NextPageToken, should.BeEmpty)
			fmt.Printf("resp.FailureOnlyRanges: %+v\n", resp.FailureOnlyRanges)
			assert.Loosely(t, resp.FailureOnlyRanges, should.Resemble([]*pb.CompareArtifactLinesResponse_FailureOnlyRange{
				{StartLine: 1, EndLine: 2, StartByte: 7, EndByte: 14},  // "line b\n"
				{StartLine: 3, EndLine: 5, StartByte: 21, EndByte: 37}, // "line d1\nline d2\n"
				{StartLine: 6, EndLine: 7, StartByte: 44, EndByte: 50}, // "line f"
			}))
		})

		t.Run("identifies failure ranges correctly, with content", func(t *ftt.Test) {
			reader := strings.NewReader(failingContent)
			resp, err := ProcessFailingReader(context.Background(), reader, passingHashes, pb.CompareArtifactLinesRequest_RANGES_WITH_CONTENT, 1000, 0, 0)

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.NextPageToken, should.BeEmpty)
			assert.Loosely(t, resp.FailureOnlyRanges, should.Resemble([]*pb.CompareArtifactLinesResponse_FailureOnlyRange{
				{StartLine: 1, EndLine: 2, StartByte: 7, EndByte: 14, Lines: []string{"line b"}},
				{StartLine: 3, EndLine: 5, StartByte: 21, EndByte: 37, Lines: []string{"line d1", "line d2"}},
				{StartLine: 6, EndLine: 7, StartByte: 44, EndByte: 50, Lines: []string{"line f"}},
			}))
		})

		t.Run("paginates by page size", func(t *ftt.Test) {
			reader := strings.NewReader(failingContent)
			// Page size of 2 should return the first two failure ranges.
			resp, err := ProcessFailingReader(context.Background(), reader, passingHashes, pb.CompareArtifactLinesRequest_RANGES_ONLY, 2, 0, 0)

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.NextPageToken, should.NotBeEmpty)
			assert.Loosely(t, len(resp.FailureOnlyRanges), should.Equal(2))
			assert.Loosely(t, resp.FailureOnlyRanges[0].StartLine, should.Equal(1))
			assert.Loosely(t, resp.FailureOnlyRanges[1].StartLine, should.Equal(3))

			// Check that the page token points to the start of the next line.
			pt, _ := decodePageToken(resp.NextPageToken)
			assert.Loosely(t, pt.NextLineNumber, should.Equal(int32(5))) // line "e" starts at index 6
			assert.Loosely(t, pt.NextByteOffset, should.Equal(int64(37)))
		})
		t.Run("paginates by content size by generating large content", func(t *ftt.Test) {
			// This test generates content dynamically to test the hardcoded constant.
			var content strings.Builder
			passingLine := "this line passes"
			// Create a large failing line of exactly 50 KiB (including newline).
			failingLineChunk := strings.Repeat("x", 50*1024-1)

			// Determine how many full chunks fit under the limit.
			numChunksToFit := maxContentBytes / (len(failingLineChunk) + 1) // +1 for newline

			// Write chunks that should be included in the first page.
			for i := 0; i < numChunksToFit; i++ {
				content.WriteString(failingLineChunk)
				content.WriteString("\n")
			}

			// Add a passing line in between.
			content.WriteString(passingLine)
			content.WriteString("\n")
			startOfNextPageByte := content.Len()
			startOfNextPageLine := numChunksToFit + 1

			// Write one more failing chunk that should be on the next page.
			content.WriteString(failingLineChunk)
			content.WriteString("\n")

			// Create a new set of passing hashes for this specific test case.
			localPassingHashes := make(map[int64]struct{})
			localPassingHashes[mustHash(passingLine)] = struct{}{}

			reader := strings.NewReader(content.String())
			resp, err := ProcessFailingReader(context.Background(), reader, localPassingHashes, pb.CompareArtifactLinesRequest_RANGES_WITH_CONTENT, 1000, 0, 0)

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.NextPageToken, should.NotBeEmpty)

			// The response should contain one large range with exactly numChunksToFit lines.
			assert.Loosely(t, len(resp.FailureOnlyRanges), should.Equal(1))
			assert.Loosely(t, len(resp.FailureOnlyRanges[0].Lines), should.Equal(numChunksToFit))
			assert.Loosely(t, resp.FailureOnlyRanges[0].Lines[0], should.Equal(failingLineChunk))

			pt, _ := decodePageToken(resp.NextPageToken)
			// Token should point to the start of the line that exceeded the content limit.
			assert.Loosely(t, pt.NextLineNumber, should.Equal(int32(startOfNextPageLine)))
			assert.Loosely(t, pt.NextByteOffset, should.Equal(int64(startOfNextPageByte)))
		})
		t.Run("File ends without a trailing newline", func(t *ftt.Test) {
			// This tests that the final `remainder` buffer is processed correctly.
			failingContentNoNewline := strings.TrimSuffix(failingContent, "\n")
			reader := strings.NewReader(failingContentNoNewline)
			resp, err := ProcessFailingReader(context.Background(), reader, passingHashes, pb.CompareArtifactLinesRequest_RANGES_ONLY, 1000, 0, 0)

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.NextPageToken, should.BeEmpty)
			// The byte offsets for the final range should be different.
			assert.Loosely(t, resp.FailureOnlyRanges, should.Resemble([]*pb.CompareArtifactLinesResponse_FailureOnlyRange{
				{StartLine: 1, EndLine: 2, StartByte: 7, EndByte: 14},  // "line b\n"
				{StartLine: 3, EndLine: 5, StartByte: 21, EndByte: 37}, // "line d1\nline d2\n"
				{StartLine: 6, EndLine: 7, StartByte: 44, EndByte: 50}, // "line f" (no trailing newline)
			}))
		})

		t.Run("File starts with a failure range", func(t *ftt.Test) {
			content := `FAILING LINE
line a
line c`
			reader := strings.NewReader(content)
			resp, err := ProcessFailingReader(context.Background(), reader, passingHashes, pb.CompareArtifactLinesRequest_RANGES_ONLY, 1000, 0, 0)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(resp.FailureOnlyRanges), should.Equal(1))
			assert.Loosely(t, resp.FailureOnlyRanges[0], should.Resemble(&pb.CompareArtifactLinesResponse_FailureOnlyRange{
				StartLine: 0, EndLine: 1, StartByte: 0, EndByte: 13,
			}))
		})

		t.Run("File with only failing lines", func(t *ftt.Test) {
			content := "line 1\nline 2\nline 3"
			reader := strings.NewReader(content)
			// Use an empty passingHashes map.
			resp, err := ProcessFailingReader(context.Background(), reader, make(map[int64]struct{}), pb.CompareArtifactLinesRequest_RANGES_ONLY, 1000, 0, 0)
			assert.Loosely(t, err, should.BeNil)
			// Should produce one single range spanning the whole file.
			assert.Loosely(t, len(resp.FailureOnlyRanges), should.Equal(1))
			assert.Loosely(t, resp.FailureOnlyRanges[0], should.Resemble(&pb.CompareArtifactLinesResponse_FailureOnlyRange{
				StartLine: 0, EndLine: 3, StartByte: 0, EndByte: 20,
			}))
		})

		t.Run("Empty file", func(t *ftt.Test) {
			reader := strings.NewReader("")
			resp, err := ProcessFailingReader(context.Background(), reader, passingHashes, pb.CompareArtifactLinesRequest_RANGES_ONLY, 1000, 0, 0)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.FailureOnlyRanges, should.BeEmpty)
			assert.Loosely(t, resp.NextPageToken, should.BeEmpty)
		})

		t.Run("File with only passing lines", func(t *ftt.Test) {
			content := "line a\nline c\nline e"
			reader := strings.NewReader(content)
			resp, err := ProcessFailingReader(context.Background(), reader, passingHashes, pb.CompareArtifactLinesRequest_RANGES_ONLY, 1000, 0, 0)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.FailureOnlyRanges, should.BeEmpty)
			assert.Loosely(t, resp.NextPageToken, should.BeEmpty)
		})
	})
}

// mustHash is a test helper that computes a hash and panics on error.
func mustHash(line string) int64 {
	h, err := hashLine([]byte(line))
	if err != nil {
		panic(err)
	}
	return h
}

// decodePageToken is a test helper.
func decodePageToken(tok string) (*pageToken, error) {
	b, err := base64.StdEncoding.DecodeString(tok)
	if err != nil {
		return nil, err
	}
	pt := &pageToken{}
	if err := json.Unmarshal(b, pt); err != nil {
		return nil, err
	}
	return pt, nil
}
