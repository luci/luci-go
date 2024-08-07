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

	_ "go.chromium.org/luci/server/tq/txn/spanner"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestToBatches(t *testing.T) {
	Convey(`ToBatches`, t, func() {
		Convey(`Non-empty`, func() {
			var rows []proto.Message
			for i := 0; i < 10; i++ {
				// Rows of ~1 MB each.
				row := &bqpb.TestVerdictRow{
					TestId: strings.Repeat("a", 999950),
				}
				So(proto.Size(row), ShouldEqual, 999954)
				rows = append(rows, row)
			}

			result, err := toBatches(rows)
			So(err, ShouldBeNil)
			// ~9 MB in the first batch.
			So(result[0], ShouldHaveLength, 9)
			// The rest of the rows in the remaining batch.
			So(result[1], ShouldHaveLength, 1)
		})
		Convey(`Empty`, func() {
			result, err := toBatches(nil)
			So(err, ShouldBeNil)
			So(result, ShouldHaveLength, 0)
		})
		Convey(`Single row too large`, func() {
			// 10 MB row.
			row := &bqpb.TestVerdictRow{
				TestId: strings.Repeat("a", 10*1000*1000),
			}
			rows := []proto.Message{row}
			_, err := toBatches(rows)
			So(err, ShouldErrLike, "a single row exceeds the maximum BigQuery AppendRows request size of 9000000 bytes")
		})
	})
}
