// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/luci/luci-go/common/logdog/types"
	. "github.com/luci/luci-go/common/testing/assertions"
	"github.com/luci/luci-go/server/logdog/storage"
	. "github.com/smartystreets/goconvey/convey"
)

func numRec(i types.MessageIndex) *rec {
	buf := bytes.Buffer{}
	binary.Write(&buf, binary.BigEndian, i)
	return &rec{
		index: i,
		data:  buf.Bytes(),
	}
}

// index builds an index of stream message number to array offset.
func index(recs []*rec) map[types.MessageIndex]int {
	index := map[types.MessageIndex]int{}
	for i, r := range recs {
		index[r.index] = i
	}
	return index
}

func TestBigTable(t *testing.T) {
	t.Parallel()

	Convey(`A memory Storage instance.`, t, func() {
		st := Storage{}
		defer st.Close()

		path := types.StreamPath("testing/+/foo/bar")

		Convey(`Can Put() log stream records {0..5, 7, 8, 10}.`, func() {
			ridx := []types.MessageIndex{0, 1, 2, 3, 4, 5, 7, 8, 10}
			for _, idx := range ridx {
				req := storage.PutRequest{
					Path:  path,
					Index: idx,
					Value: numRec(idx).data,
				}

				So(st.Put(&req), ShouldBeNil)
			}

			// Forward-indexed records.
			recs := make([]*rec, len(ridx))
			for i, idx := range ridx {
				recs[i] = numRec(idx)
			}

			var getRecs []*rec
			getAllCB := func(idx types.MessageIndex, data []byte) bool {
				getRecs = append(getRecs, &rec{
					index: idx,
					data:  data,
				})
				return true
			}

			Convey(`Put()`, func() {
				Convey(`Will return ErrExists when putting an existing entry.`, func() {
					req := storage.PutRequest{
						Path:  path,
						Index: 5,
						Value: []byte("ohai"),
					}

					So(st.Put(&req), ShouldEqual, storage.ErrExists)
				})
			})

			Convey(`Get()`, func() {
				Convey(`Can retrieve all of the records correctly.`, func() {
					recs := make([]*rec, len(ridx))
					for i, idx := range ridx {
						recs[i] = numRec(idx)
					}

					req := storage.GetRequest{
						Path: path,
					}

					So(st.Get(&req, getAllCB), ShouldBeNil)
					So(getRecs, ShouldResembleV, recs)
				})

				Convey(`Will adhere to GetRequest limit.`, func() {
					req := storage.GetRequest{
						Path:  path,
						Limit: 4,
					}

					So(st.Get(&req, getAllCB), ShouldBeNil)
					So(getRecs, ShouldResembleV, recs[:4])
				})

				Convey(`Will adhere to hard limit.`, func() {
					st.MaxGetCount = 3
					req := storage.GetRequest{
						Path:  path,
						Limit: 4,
					}

					So(st.Get(&req, getAllCB), ShouldBeNil)
					So(getRecs, ShouldResembleV, recs[:3])
				})

				Convey(`Will stop iterating if callback returns false.`, func() {
					req := storage.GetRequest{
						Path: path,
					}

					count := 0
					err := st.Get(&req, func(types.MessageIndex, []byte) bool {
						count++
						return false
					})
					So(err, ShouldBeNil)
					So(count, ShouldEqual, 1)
				})

				Convey(`Will fail to retrieve records if the stream doesn't exist.`, func() {
					req := storage.GetRequest{
						Path: "testing/+/does/not/exist",
					}

					So(st.Get(&req, getAllCB), ShouldEqual, storage.ErrDoesNotExist)
				})
			})

			Convey(`Tail()`, func() {
				// Build reverse record set.
				rrecs := make([]*rec, len(recs))
				for i, r := range recs {
					rrecs[len(recs)-i-1] = r
				}
				ridx := index(rrecs)

				Convey(`Can retrieve all of the records correctly.`, func() {
					req := storage.GetRequest{
						Path: path,
					}

					So(st.Tail(&req, getAllCB), ShouldBeNil)
					So(getRecs, ShouldResembleV, rrecs[ridx[5]:])
				})

				Convey(`Can retrieve records 7 correctly.`, func() {
					req := storage.GetRequest{
						Path:  path,
						Index: 7,
					}

					So(st.Tail(&req, getAllCB), ShouldBeNil)
					So(getRecs, ShouldResembleV, []*rec{rrecs[ridx[8]], rrecs[ridx[7]]})
				})

				Convey(`Returns no records when indexed at 9.`, func() {
					req := storage.GetRequest{
						Path:  path,
						Index: 9,
					}

					So(st.Tail(&req, getAllCB), ShouldBeNil)
					So(getRecs, ShouldResembleV, []*rec(nil))
				})

				Convey(`Will adhere to GetRequest limit.`, func() {
					req := storage.GetRequest{
						Path:  path,
						Limit: 4, // Only look "4" records ahead for end-of-stream.
					}

					So(st.Tail(&req, getAllCB), ShouldBeNil)
					So(getRecs, ShouldResembleV, rrecs[ridx[3]:])
				})

				Convey(`Will adhere to hard limit.`, func() {
					st.MaxGetCount = 3 // Only look "3" records ahead for end-of-stream.
					req := storage.GetRequest{
						Path:  path,
						Limit: 4,
					}

					So(st.Tail(&req, getAllCB), ShouldBeNil)
					So(getRecs, ShouldResembleV, rrecs[ridx[2]:])
				})

				Convey(`Will stop iterating if callback returns false.`, func() {
					req := storage.GetRequest{
						Path: path,
					}

					count := 0
					err := st.Tail(&req, func(types.MessageIndex, []byte) bool {
						count++
						return false
					})
					So(err, ShouldBeNil)
					So(count, ShouldEqual, 1)
				})

				Convey(`Will fail to retrieve records if the stream doesn't exist.`, func() {
					req := storage.GetRequest{
						Path: "testing/+/does/not/exist",
					}

					So(st.Tail(&req, getAllCB), ShouldEqual, storage.ErrDoesNotExist)
				})
			})

			Convey(`Purge()`, func() {
				Convey(`Can purge the test stream.`, func() {
					So(st.Purge(path), ShouldBeNil)

					req := storage.GetRequest{
						Path: path,
					}
					So(st.Get(&req, getAllCB), ShouldEqual, storage.ErrDoesNotExist)
				})

				Convey(`Will error if purging a non-existent stream.`, func() {
					So(st.Purge("testing/+/does/not/exist"), ShouldEqual, storage.ErrDoesNotExist)
				})
			})
		})
	})
}
