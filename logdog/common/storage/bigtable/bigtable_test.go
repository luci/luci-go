// Copyright 2015 The LUCI Authors.
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

package bigtable

import (
	"testing"

	"go.chromium.org/luci/logdog/common/storage"

	"cloud.google.com/go/bigtable"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBigTable(t *testing.T) {
	t.Parallel()

	Convey(`Testing BigTable internal functions`, t, func() {
		s := NewMemoryInstance(nil)
		defer s.Close()

		Convey(`Given a fake BigTable row`, func() {
			fakeRow := bigtable.Row{
				"log": []bigtable.ReadItem{
					{
						Row:    "testrow",
						Column: logColName,
						Value:  []byte("here is my data"),
					},
				},
			}

			Convey(`Can extract log data.`, func() {
				d, err := getLogRowData(fakeRow)
				So(err, ShouldBeNil)
				So(d, ShouldResemble, []byte("here is my data"))
			})

			Convey(`Will fail to extract if the column is missing.`, func() {
				fakeRow["log"][0].Column = "not-data"

				_, err := getLogRowData(fakeRow)
				So(err, ShouldEqual, storage.ErrDoesNotExist)
			})

			Convey(`Will fail to extract if the family does not exist.`, func() {
				So(getReadItem(fakeRow, "invalid", "invalid"), ShouldBeNil)
			})

			Convey(`Will fail to extract if the column does not exist.`, func() {
				So(getReadItem(fakeRow, "log", "invalid"), ShouldBeNil)
			})
		})

	})
}
