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

package bq

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	bqpb "go.chromium.org/luci/analysis/proto/bq"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	_ "go.chromium.org/luci/server/tq/txn/spanner"
)

func TestToBatches(t *testing.T) {
	ftt.Run(`ToBatches`, t, func(t *ftt.Test) {
		t.Run(`Non-empty`, func(t *ftt.Test) {
			var rows []proto.Message
			for range 10 {
				// Rows of ~1 MB each.
				row := &bqpb.TestVerdictRow{
					TestId: strings.Repeat("a", 999950),
				}
				assert.Loosely(t, proto.Size(row), should.Equal(999954))
				rows = append(rows, row)
			}

			result, err := toBatches(rows)
			assert.Loosely(t, err, should.BeNil)
			// ~9 MB in the first batch.
			assert.Loosely(t, result[0], should.HaveLength(9))
			// The rest of the rows in the remaining batch.
			assert.Loosely(t, result[1], should.HaveLength(1))
		})
		t.Run(`Empty`, func(t *ftt.Test) {
			result, err := toBatches(nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.HaveLength(0))
		})
		t.Run(`Single row too large`, func(t *ftt.Test) {
			// 10 MB row.
			row := &bqpb.TestVerdictRow{
				TestId: strings.Repeat("a", 10*1000*1000),
			}
			rows := []proto.Message{row}
			_, err := toBatches(rows)
			assert.Loosely(t, err, should.ErrLike("a single row exceeds the maximum BigQuery AppendRows request size of 9000000 bytes"))
		})
	})
}
