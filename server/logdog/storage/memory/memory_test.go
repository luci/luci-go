// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/server/logdog/storage"

	. "github.com/luci/luci-go/common/testing/assertions"
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

				Convey(`Will return an error if one is set.`, func() {
					st.SetErr(errors.New("test error"))

					req := storage.PutRequest{
						Path:  path,
						Index: 1337,
					}
					So(st.Put(&req), ShouldErrLike, "test error")
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
					So(getRecs, ShouldResemble, recs)
				})

				Convey(`Will adhere to GetRequest limit.`, func() {
					req := storage.GetRequest{
						Path:  path,
						Limit: 4,
					}

					So(st.Get(&req, getAllCB), ShouldBeNil)
					So(getRecs, ShouldResemble, recs[:4])
				})

				Convey(`Will adhere to hard limit.`, func() {
					st.MaxGetCount = 3
					req := storage.GetRequest{
						Path:  path,
						Limit: 4,
					}

					So(st.Get(&req, getAllCB), ShouldBeNil)
					So(getRecs, ShouldResemble, recs[:3])
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

				Convey(`Will return an error if one is set.`, func() {
					st.SetErr(errors.New("test error"))

					req := storage.GetRequest{
						Path: path,
					}
					So(st.Get(&req, nil), ShouldErrLike, "test error")
				})
			})

			Convey(`Tail()`, func() {
				Convey(`Can retrieve the tail record, 10.`, func() {
					d, idx, err := st.Tail(path)
					So(err, ShouldBeNil)
					So(d, ShouldResemble, numRec(10).data)
					So(idx, ShouldEqual, 10)
				})

				Convey(`Will fail to retrieve records if the stream doesn't exist.`, func() {
					_, _, err := st.Tail("testing/+/does/not/exist")
					So(err, ShouldEqual, storage.ErrDoesNotExist)
				})

				Convey(`Will return an error if one is set.`, func() {
					st.SetErr(errors.New("test error"))
					_, _, err := st.Tail("")
					So(err, ShouldErrLike, "test error")
				})
			})

			Convey(`Config()`, func() {
				cfg := storage.Config{
					MaxLogAge: time.Hour,
				}

				Convey(`Can update the configuration.`, func() {
					So(st.Config(cfg), ShouldBeNil)
					So(st.MaxLogAge, ShouldEqual, cfg.MaxLogAge)
				})

				Convey(`Will return an error if one is set.`, func() {
					st.SetErr(errors.New("test error"))
					So(st.Config(storage.Config{}), ShouldErrLike, "test error")
				})
			})

			Convey(`Errors can be set, cleared, and set again.`, func() {
				So(st.Config(storage.Config{}), ShouldBeNil)

				st.SetErr(errors.New("test error"))
				So(st.Config(storage.Config{}), ShouldErrLike, "test error")

				st.SetErr(nil)
				So(st.Config(storage.Config{}), ShouldBeNil)

				st.SetErr(errors.New("test error"))
				So(st.Config(storage.Config{}), ShouldErrLike, "test error")
			})
		})
	})
}
