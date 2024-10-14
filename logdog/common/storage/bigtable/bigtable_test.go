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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/logdog/common/storage"

	"cloud.google.com/go/bigtable"
)

func TestBigTable(t *testing.T) {
	t.Parallel()

	ftt.Run(`Testing BigTable internal functions`, t, func(t *ftt.Test) {
		s := NewMemoryInstance(nil)
		defer s.Close()

		t.Run(`Given a fake BigTable row`, func(t *ftt.Test) {
			fakeRow := bigtable.Row{
				"log": []bigtable.ReadItem{
					{
						Row:    "testrow",
						Column: logColName,
						Value:  []byte("here is my data"),
					},
				},
			}

			t.Run(`Can extract log data.`, func(t *ftt.Test) {
				d, err := getLogRowData(fakeRow)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, d, should.Resemble([]byte("here is my data")))
			})

			t.Run(`Will fail to extract if the column is missing.`, func(t *ftt.Test) {
				fakeRow["log"][0].Column = "not-data"

				_, err := getLogRowData(fakeRow)
				assert.Loosely(t, err, should.Equal(storage.ErrDoesNotExist))
			})

			t.Run(`Will fail to extract if the family does not exist.`, func(t *ftt.Test) {
				assert.Loosely(t, getReadItem(fakeRow, "invalid", "invalid"), should.BeNil)
			})

			t.Run(`Will fail to extract if the column does not exist.`, func(t *ftt.Test) {
				assert.Loosely(t, getReadItem(fakeRow, "log", "invalid"), should.BeNil)
			})
		})

	})
}
