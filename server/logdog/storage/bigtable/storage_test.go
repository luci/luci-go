// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bigtable

import (
	"bytes"
	"testing"

	"github.com/luci/gkvlite"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/server/logdog/storage"
	"golang.org/x/net/context"
	"google.golang.org/cloud/bigtable"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

// errDontReallyDelete is a sentinel error. If btTableTest's deleteErr is
// configured with this error, the deleteRow operation will not actually delete
// the row, but will still return success.
var errDontReallyDelete = errors.New("don't really delete")

// btTableTest is an in-memory implementation of btTable interface for testing.
//
// This is a simple implementation; not an efficient one.
type btTableTest struct {
	s *gkvlite.Store
	c *gkvlite.Collection

	// err, if true, is the error immediately returned by functions.
	err error
	// deleteErr, if not nil, is the error returned by the deleteRow method.
	deleteErr error
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
	prefix := rk.pathPrefix()
	var ierr error
	err := t.collection().VisitItemsAscend(enc, !keysOnly, func(i *gkvlite.Item) bool {
		var drk *rowKey
		drk, ierr = decodeRowKey(string(i.Key))
		if ierr != nil {
			return false
		}
		if drk.pathPrefix() != prefix {
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

func (t *btTableTest) deleteRow(c context.Context, rk *rowKey) error {
	real := true
	switch err := t.deleteErr; err {
	case errDontReallyDelete:
		real = false
	case nil:
		break
	default:
		return err
	}

	if real {
		if ok, err := t.c.Delete([]byte(rk.encode())); err != nil {
			return err
		} else if !ok {
			return errors.New("row does not exist")
		}
	}
	return nil
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

		put := func(path string, index int, d string) error {
			return s.Put(&storage.PutRequest{
				Path:  types.StreamPath(path),
				Index: types.MessageIndex(index),
				Value: []byte(d),
			})
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
					So(bt.dataMap(), ShouldResemble, map[string][]byte{
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
					So(got, ShouldResemble, []string{"0", "1", "2"})
				})

				Convey(`Will fetch A{1, 2} with when index=1 and limit=2.`, func() {
					got, err := get("A", 1, 2)
					So(err, ShouldBeNil)
					So(got, ShouldResemble, []string{"1", "2"})
				})

				Convey(`Will fetch B{10, 12, 13} for B.`, func() {
					got, err := get("B", 0, 0)
					So(err, ShouldBeNil)
					So(got, ShouldResemble, []string{"10", "12", "13"})
				})

				Convey(`Will fetch B{12, 13} when index=11.`, func() {
					got, err := get("B", 11, 0)
					So(err, ShouldBeNil)
					So(got, ShouldResemble, []string{"12", "13"})
				})

				Convey(`Will fetch {} for INVALID.`, func() {
					got, err := get("INVALID", 0, 0)
					So(err, ShouldBeNil)
					So(got, ShouldResemble, []string{})
				})
			})

			Convey(`Testing "Tail"...`, func() {
				tail := func(path string) (string, error) {
					got, _, err := s.Tail(types.StreamPath(path))
					return string(got), err
				}

				Convey(`A tail request for "A" returns A{2, 1, 0}.`, func() {
					got, err := tail("A")
					So(err, ShouldBeNil)
					So(got, ShouldEqual, "2")
				})

				Convey(`A tail request for "B" returns B{13}.`, func() {
					got, err := tail("B")
					So(err, ShouldBeNil)
					So(got, ShouldEqual, "13")
				})

				Convey(`A tail request for "INVALID" errors NOT FOUND.`, func() {
					_, err := tail("INVALID")
					So(err, ShouldEqual, storage.ErrDoesNotExist)
				})
			})

			Convey(`Testing "Purge"...`, func() {
				Convey(`Can purge log stream "A", then "B".`, func() {
					// Purge "A".
					So(s.Purge(types.StreamPath("A")), ShouldBeNil)

					got, err := get("A", 0, 0)
					So(err, ShouldBeNil)
					So(got, ShouldResemble, []string{})

					got, err = get("B", 0, 0)
					So(err, ShouldBeNil)
					So(got, ShouldResemble, []string{"10", "12", "13"})

					// Now purge "B".
					So(s.Purge(types.StreamPath("B")), ShouldBeNil)

					got, err = get("B", 0, 0)
					So(err, ShouldBeNil)
					So(got, ShouldResemble, []string{})
				})

				Convey(`Will return an error if Storage failed to purge a row.`, func() {
					bt.deleteErr = errors.New("testing error")

					err := s.Purge(types.StreamPath("A"))
					So(err, ShouldErrLike, "testing error")
					So(errors.IsTransient(err), ShouldBeFalse)
				})

				Convey(`Will return a transient error if Storage transiently failed to purge a row.`, func() {
					bt.deleteErr = errors.WrapTransient(errors.New("testing error"))

					err := s.Purge(types.StreamPath("A"))
					So(err, ShouldErrLike, "testing error")
					So(errors.IsTransient(err), ShouldBeTrue)
				})

				Convey(`Will return an error if there are still rows left after delete.`, func() {
					bt.deleteErr = errDontReallyDelete
					So(s.Purge(types.StreamPath("A")), ShouldErrLike, "encountered row data post-purge")
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
