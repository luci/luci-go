// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bigtable

import (
	"bytes"
	"testing"

	"github.com/luci/gkvlite"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/testing/assertions"
	"github.com/luci/luci-go/server/logdog/storage"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
	"google.golang.org/cloud/bigtable"
)

// btTableTest is an in-memory implementation of btTable interface for testing.
//
// This is a simple implementation; not an efficient one.
type btTableTest struct {
	s *gkvlite.Store
	c *gkvlite.Collection

	// err, if true, is the error immediately returned by functions.
	err error
}

func (t *btTableTest) close() {
	if t.s != nil {
		t.s.Close()
		t.s = nil
	}
}

func (t *btTableTest) collection() *gkvlite.Collection {
	if t.s == nil {
		var err error
		t.s, err = gkvlite.NewStore(nil)
		if err != nil {
			panic(err)
		}
		t.c = t.s.MakePrivateCollection(bytes.Compare)
	}
	return t.c
}

func (t *btTableTest) putLogData(c context.Context, rk *rowKey, d []byte) error {
	if t.err != nil {
		return t.err
	}

	enc := []byte(rk.encode())
	coll := t.collection()
	if item, _ := coll.Get(enc); item != nil {
		return storage.ErrExists
	}

	if err := coll.Set(enc, d); err != nil {
		panic(err)
	}
	return nil
}

func (t *btTableTest) getLogData(c context.Context, rk *rowKey, limit int, keysOnly bool, cb btGetCallback) error {
	if t.err != nil {
		return t.err
	}

	enc := []byte(rk.encode())
	var ierr error
	err := t.collection().VisitItemsAscend(enc, !keysOnly, func(i *gkvlite.Item) bool {
		var drk *rowKey
		drk, ierr = decodeRowKey(string(i.Key))
		if ierr != nil {
			return false
		}

		if ierr = cb(drk, i.Val); ierr != nil {
			if ierr == errStop {
				ierr = nil
			}
			return false
		}

		if limit > 0 {
			limit--
			if limit == 0 {
				return false
			}
		}

		return true
	})
	if err != nil {
		panic(err)
	}
	return ierr
}

func (t *btTableTest) dataMap() map[string][]byte {
	result := map[string][]byte{}

	err := t.collection().VisitItemsAscend([]byte(nil), true, func(i *gkvlite.Item) bool {
		result[string(i.Key)] = i.Val
		return true
	})
	if err != nil {
		panic(err)
	}
	return result
}

func TestStorage(t *testing.T) {
	t.Parallel()

	Convey(`A BigTable storage instance bound to a testing BigTable instance`, t, func() {
		bt := btTableTest{}
		defer bt.close()

		s := btStorage{
			Options: &Options{
				Project:  "test-project",
				Zone:     "test-zone",
				Cluster:  "test-cluster",
				LogTable: "test-log-table",
			},
			ctx:    context.Background(),
			client: nil,
			table:  &bt,
		}

		put := func(path string, index int, d string) error {
			return s.Put(&storage.PutRequest{
				Path:  types.StreamPath(path),
				Index: types.MessageIndex(index),
				Value: []byte(d),
			})
		}

		get := func(path string, index int, limit int) ([]string, error) {
			req := storage.GetRequest{
				Path:  types.StreamPath(path),
				Index: types.MessageIndex(index),
				Limit: limit,
			}
			got := []string{}
			err := s.Get(&req, func(idx types.MessageIndex, d []byte) bool {
				got = append(got, string(d))
				return true
			})
			return got, err
		}

		tail := func(path string, index int, limit int) ([]string, error) {
			req := storage.GetRequest{
				Path:  types.StreamPath(path),
				Index: types.MessageIndex(index),
				Limit: limit,
			}
			got := []string{}
			err := s.Tail(&req, func(idx types.MessageIndex, d []byte) bool {
				got = append(got, string(d))
				return true
			})
			return got, err
		}

		Convey(`With row data: A{0, 1, 2}, B{10, 12, 13}`, func() {
			So(put("A", 0, "0"), ShouldBeNil)
			So(put("A", 1, "1"), ShouldBeNil)
			So(put("A", 2, "2"), ShouldBeNil)
			So(put("B", 10, "10"), ShouldBeNil)
			So(put("B", 12, "12"), ShouldBeNil)
			So(put("B", 13, "13"), ShouldBeNil)

			ekey := func(p string, v int64) string {
				return newRowKey(p, v).encode()
			}

			Convey(`Testing "Put"...`, func() {
				Convey(`Loads the row data.`, func() {
					So(bt.dataMap(), assertions.ShouldResembleV, map[string][]byte{
						ekey("A", 0):  []byte("0"),
						ekey("A", 1):  []byte("1"),
						ekey("A", 2):  []byte("2"),
						ekey("B", 10): []byte("10"),
						ekey("B", 12): []byte("12"),
						ekey("B", 13): []byte("13"),
					})
				})
			})

			Convey(`Testing "Get"...`, func() {
				Convey(`Can fetch the full row, "A".`, func() {
					got, err := get("A", 0, 0)
					So(err, ShouldBeNil)
					So(got, assertions.ShouldResembleV, []string{"0", "1", "2"})
				})

				Convey(`Will fetch A{1, 2} with when index=1 and limit=2.`, func() {
					got, err := get("A", 1, 2)
					So(err, ShouldBeNil)
					So(got, assertions.ShouldResembleV, []string{"1", "2"})
				})

				Convey(`Will fetch B{10, 12, 13} for B.`, func() {
					got, err := get("B", 0, 0)
					So(err, ShouldBeNil)
					So(got, assertions.ShouldResembleV, []string{"10", "12", "13"})
				})

				Convey(`Will fetch B{12, 13} when index=11.`, func() {
					got, err := get("B", 11, 0)
					So(err, ShouldBeNil)
					So(got, assertions.ShouldResembleV, []string{"12", "13"})
				})

				Convey(`Will fetch {} for INVALID.`, func() {
					got, err := get("INVALID", 0, 0)
					So(err, ShouldBeNil)
					So(got, assertions.ShouldResembleV, []string{})
				})
			})

			Convey(`Testing "Tail"...`, func() {
				Convey(`A tail request for "A" returns A{2, 1, 0}.`, func() {
					got, err := tail("A", 0, 0)
					So(err, ShouldBeNil)
					So(got, assertions.ShouldResembleV, []string{"2", "1", "0"})
				})

				Convey(`A tail request for "A" with limit 2 returns A{1, 0}.`, func() {
					got, err := tail("A", 0, 2)
					So(err, ShouldBeNil)
					So(got, assertions.ShouldResembleV, []string{"1", "0"})
				})

				Convey(`A tail request for "B" with index 10 returns B{10}.`, func() {
					got, err := tail("B", 10, 0)
					So(err, ShouldBeNil)
					So(got, assertions.ShouldResembleV, []string{"10"})
				})

				Convey(`A tail request for "B" with index 11 errors NOT FOUND.`, func() {
					_, err := tail("B", 11, 0)
					So(err, ShouldEqual, storage.ErrDoesNotExist)
				})

				Convey(`A tail request for "B" with index 12 returns B{13, 12}.`, func() {
					got, err := tail("B", 12, 0)
					So(err, ShouldBeNil)
					So(got, assertions.ShouldResembleV, []string{"13", "12"})
				})

				Convey(`A tail request for "INVALID" errors NOT FOUND.`, func() {
					_, err := tail("INVALID", 0, 0)
					So(err, ShouldEqual, storage.ErrDoesNotExist)
				})
			})

			Convey(`Testing "Purge"...`, func() {
				Convey(`Is not implemented, and should panic.`, func() {
					So(func() { s.Purge("") }, ShouldPanic)
				})
			})
		})

		Convey(`Given a fake BigTable row`, func() {
			fakeRow := bigtable.Row{
				"log": []bigtable.ReadItem{
					{
						Row:    "testrow",
						Column: "log:data",
						Value:  []byte("here is my data"),
					},
				},
			}

			Convey(`Can extract log data.`, func() {
				d, err := getLogData(fakeRow)
				So(err, ShouldBeNil)
				So(d, ShouldResemble, []byte("here is my data"))
			})

			Convey(`Will fail to extract if the column is missing.`, func() {
				fakeRow["log"][0].Column = "not-data"
				_, err := getLogData(fakeRow)
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
