// Copyright 2024 The LUCI Authors.
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
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestToFailureOnlyLineRanges(t *testing.T) {
	contentString := `2024-05-06T05:58:57.490076Z ERROR test[9617:9617]: log line 1
2024-05-06T05:58:57.491037Z VERBOSE1 test[9617:9617]: [file.cc(845)] log line 2
2024-05-06T05:58:57.577095Z WARNING test[9617:9617]: [file.cc(89)] log line 3.
2024-05-06T05:58:57.577324Z INFO test[9617:9617]: [file.cc(140)] log line 4 {
	log line no timestamp
}`

	ftt.Run(`ToFailureOnlyLineRanges`, t, func(t *ftt.Test) {
		t.Run(`given a list of passing hashes, should return correct in_passes fields`, func(t *ftt.Test) {
			contentBytes := []byte(contentString)
			passingHashes := map[int64]struct{}{}
			for i, line := range bytes.Split(contentBytes, []byte("\n")) {
				if i%2 == 0 {
					passingHashes[HashLine(string(line))] = struct{}{}
				}
			}
			ranges, err := ToFailureOnlyLineRanges("log.text", "text/log", contentBytes, passingHashes, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(ranges), should.Equal(3))
			assert.Loosely(t, ranges, should.Resemble([]*pb.QueryArtifactFailureOnlyLinesResponse_LineRange{
				{Start: 1, End: 2},
				{Start: 3, End: 4},
				{Start: 5, End: 6},
			}))
		})
	})
}
