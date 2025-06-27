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

package memory

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/logdog/common/storage"
	"go.chromium.org/luci/logdog/common/types"
)

func numRec(v types.MessageIndex) *rec {
	buf := bytes.Buffer{}
	binary.Write(&buf, binary.BigEndian, v)
	return &rec{
		index: v,
		data:  buf.Bytes(),
	}
}

func mustGetIndex(e *storage.Entry) types.MessageIndex {
	idx, err := e.GetStreamIndex()
	if err != nil {
		panic(err)
	}
	return idx
}

func TestBigTable(t *testing.T) {
	t.Parallel()

	ftt.Run(`A memory Storage instance.`, t, func(t *ftt.Test) {
		c := context.TODO()

		st := Storage{}
		defer st.Close()

		project := "test-project"
		path := types.StreamPath("testing/+/foo/bar")

		t.Run(`Can Put() log stream records {0..5, 7, 8, 10}.`, func(t *ftt.Test) {
			var indices []types.MessageIndex

			putRange := func(start types.MessageIndex, count int) error {
				req := storage.PutRequest{
					Project: project,
					Path:    path,
					Index:   start,
				}
				for i := range count {
					index := start + types.MessageIndex(i)
					req.Values = append(req.Values, numRec(index).data)
					indices = append(indices, index)
				}
				return st.Put(c, req)
			}

			assert.Loosely(t, putRange(0, 6), should.BeNil)
			assert.Loosely(t, putRange(7, 2), should.BeNil)
			assert.Loosely(t, putRange(10, 1), should.BeNil)

			// Forward-indexed records.
			recs := make([]*rec, len(indices))
			for i, idx := range indices {
				recs[i] = numRec(idx)
			}

			var getRecs []*rec
			getAllCB := func(e *storage.Entry) bool {
				getRecs = append(getRecs, &rec{
					index: mustGetIndex(e),
					data:  e.D,
				})
				return true
			}

			t.Run(`Put()`, func(t *ftt.Test) {
				req := storage.PutRequest{
					Project: project,
					Path:    path,
				}

				t.Run(`Will return ErrExists when putting an existing entry.`, func(t *ftt.Test) {
					req.Values = [][]byte{[]byte("ohai")}

					assert.Loosely(t, st.Put(c, req), should.Equal(storage.ErrExists))
				})

				t.Run(`Will return an error if one is set.`, func(t *ftt.Test) {
					st.SetErr(errors.New("test error"))

					req.Index = 1337
					assert.Loosely(t, st.Put(c, req), should.ErrLike("test error"))
				})
			})

			t.Run(`Get()`, func(t *ftt.Test) {
				req := storage.GetRequest{
					Project: project,
					Path:    path,
				}

				t.Run(`Can retrieve all of the records correctly.`, func(t *ftt.Test) {
					assert.Loosely(t, st.Get(c, req, getAllCB), should.BeNil)
					assert.Loosely(t, getRecs, should.Resemble(recs))
				})

				t.Run(`Will adhere to GetRequest limit.`, func(t *ftt.Test) {
					req.Limit = 4

					assert.Loosely(t, st.Get(c, req, getAllCB), should.BeNil)
					assert.Loosely(t, getRecs, should.Resemble(recs[:4]))
				})

				t.Run(`Will adhere to hard limit.`, func(t *ftt.Test) {
					st.MaxGetCount = 3
					req.Limit = 4

					assert.Loosely(t, st.Get(c, req, getAllCB), should.BeNil)
					assert.Loosely(t, getRecs, should.Resemble(recs[:3]))
				})

				t.Run(`Will stop iterating if callback returns false.`, func(t *ftt.Test) {
					count := 0
					err := st.Get(c, req, func(*storage.Entry) bool {
						count++
						return false
					})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, count, should.Equal(1))
				})

				t.Run(`Will fail to retrieve records if the project doesn't exist.`, func(t *ftt.Test) {
					req.Project = "project-does-not-exist"

					assert.Loosely(t, st.Get(c, req, getAllCB), should.Equal(storage.ErrDoesNotExist))
				})

				t.Run(`Will fail to retrieve records if the path doesn't exist.`, func(t *ftt.Test) {
					req.Path = "testing/+/does/not/exist"

					assert.Loosely(t, st.Get(c, req, getAllCB), should.Equal(storage.ErrDoesNotExist))
				})

				t.Run(`Will return an error if one is set.`, func(t *ftt.Test) {
					st.SetErr(errors.New("test error"))

					assert.Loosely(t, st.Get(c, req, nil), should.ErrLike("test error"))
				})
			})

			t.Run(`Tail()`, func(t *ftt.Test) {
				t.Run(`Can retrieve the tail record, 10.`, func(t *ftt.Test) {
					e, err := st.Tail(c, project, path)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, e.D, should.Resemble(numRec(10).data))
					assert.Loosely(t, mustGetIndex(e), should.Equal(10))
				})

				t.Run(`Will fail to retrieve records if the project doesn't exist.`, func(t *ftt.Test) {
					_, err := st.Tail(c, "project-does-not-exist", path)
					assert.Loosely(t, err, should.Equal(storage.ErrDoesNotExist))
				})

				t.Run(`Will fail to retrieve records if the path doesn't exist.`, func(t *ftt.Test) {
					_, err := st.Tail(c, project, "testing/+/does/not/exist")
					assert.Loosely(t, err, should.Equal(storage.ErrDoesNotExist))
				})

				t.Run(`Will return an error if one is set.`, func(t *ftt.Test) {
					st.SetErr(errors.New("test error"))
					_, err := st.Tail(c, "", "")
					assert.Loosely(t, err, should.ErrLike("test error"))
				})
			})

		})
	})
}
