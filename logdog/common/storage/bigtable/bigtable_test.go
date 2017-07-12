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
	"time"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/logdog/common/storage"

	"cloud.google.com/go/bigtable"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBigTable(t *testing.T) {
	t.Parallel()

	Convey(`Testing BigTable internal functions`, t, func() {
		s := NewMemoryInstance(context.Background(), Options{})
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

		Convey(`When pushing a configuration`, func() {
			cfg := storage.Config{
				MaxLogAge: 1 * time.Hour,
			}

			Convey(`Can successfully apply configuration.`, func() {
				So(s.Config(cfg), ShouldBeNil)
				So(s.MaxLogAge(), ShouldEqual, cfg.MaxLogAge)
			})

			Convey(`With return an error if the configuration fails to apply.`, func() {
				testErr := errors.New("test error")
				s.SetErr(testErr)

				So(s.Config(cfg), ShouldEqual, testErr)
			})
		})
	})
}
