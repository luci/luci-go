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
	"bytes"
	"context"
	"strconv"
	"testing"

	"go.chromium.org/luci/common/data/recordio"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/logdog/common/storage"
	"go.chromium.org/luci/logdog/common/storage/memory"
	"go.chromium.org/luci/logdog/common/types"
)

func mustGetIndex(e *storage.Entry) types.MessageIndex {
	idx, err := e.GetStreamIndex()
	if err != nil {
		panic(err)
	}
	return idx
}

func TestStorage(t *testing.T) {
	t.Parallel()

	ftt.Run(`A BigTable storage instance bound to a testing BigTable instance`, t, func(t *ftt.Test) {
		c := context.Background()

		var cache memory.Cache
		s := NewMemoryInstance(&cache)
		defer s.Close()

		project := "test-project"
		get := func(path string, index int, limit int, keysOnly bool) ([]string, error) {
			req := storage.GetRequest{
				Project:  project,
				Path:     types.StreamPath(path),
				Index:    types.MessageIndex(index),
				Limit:    limit,
				KeysOnly: keysOnly,
			}
			var got []string
			err := s.Get(c, req, func(e *storage.Entry) bool {
				if keysOnly {
					got = append(got, strconv.Itoa(int(mustGetIndex(e))))
				} else {
					got = append(got, string(e.D))
				}
				return true
			})
			return got, err
		}

		put := func(path string, index int, d ...string) error {
			data := make([][]byte, len(d))
			for i, v := range d {
				data[i] = []byte(v)
			}

			return s.Put(c, storage.PutRequest{
				Project: project,
				Path:    types.StreamPath(path),
				Index:   types.MessageIndex(index),
				Values:  data,
			})
		}

		ekey := func(path string, v, c int64) string {
			return newRowKey(project, path, v, c).encode()
		}
		records := func(s ...string) []byte {
			buf := bytes.Buffer{}
			w := recordio.NewWriter(&buf)

			for _, v := range s {
				if _, err := w.Write([]byte(v)); err != nil {
					panic(err)
				}
				if err := w.Flush(); err != nil {
					panic(err)
				}
			}

			return buf.Bytes()
		}

		t.Run(`With an artificial maximum BigTable row size of two records`, func(t *ftt.Test) {
			// Artificially constrain row size. 4 = 2*{size/1, data/1} RecordIO
			// entries.
			s.SetMaxRowSize(4)

			t.Run(`Will split row data that overflows the table into multiple rows.`, func(t *ftt.Test) {
				assert.Loosely(t, put("A", 0, "0", "1", "2", "3"), should.BeNil)

				assert.Loosely(t, s.DataMap(), should.Resemble(map[string][]byte{
					ekey("A", 1, 2): records("0", "1"),
					ekey("A", 3, 2): records("2", "3"),
				}))
			})

			t.Run(`Loading a single row data beyond the maximum row size will fail.`, func(t *ftt.Test) {
				assert.Loosely(t, put("A", 0, "0123"), should.ErrLike("single row entry exceeds maximum size"))
			})
		})

		t.Run(`With row data: A{0, 1, 2, 3, 4}, B{10, 12, 13}`, func(t *ftt.Test) {
			assert.Loosely(t, put("A", 0, "0", "1", "2"), should.BeNil)
			assert.Loosely(t, put("A", 3, "3", "4"), should.BeNil)
			assert.Loosely(t, put("B", 10, "10"), should.BeNil)
			assert.Loosely(t, put("B", 12, "12", "13"), should.BeNil)
			assert.Loosely(t, put("C", 0, "0", "1", "2"), should.BeNil)
			assert.Loosely(t, put("C", 4, "4"), should.BeNil)

			t.Run(`Testing "Put"...`, func(t *ftt.Test) {
				t.Run(`Loads the row data.`, func(t *ftt.Test) {
					assert.Loosely(t, s.DataMap(), should.Resemble(map[string][]byte{
						ekey("A", 2, 3):  records("0", "1", "2"),
						ekey("A", 4, 2):  records("3", "4"),
						ekey("B", 10, 1): records("10"),
						ekey("B", 13, 2): records("12", "13"),
						ekey("C", 2, 3):  records("0", "1", "2"),
						ekey("C", 4, 1):  records("4"),
					}))
				})
			})

			t.Run(`Testing "Get"...`, func(t *ftt.Test) {
				t.Run(`Can fetch the full row, "A".`, func(t *ftt.Test) {
					got, err := get("A", 0, 0, false)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, got, should.Resemble([]string{"0", "1", "2", "3", "4"}))
				})

				t.Run(`Will fetch A{1, 2, 3, 4} with index=1.`, func(t *ftt.Test) {
					got, err := get("A", 1, 0, false)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, got, should.Resemble([]string{"1", "2", "3", "4"}))
				})

				t.Run(`Will fetch A{1, 2} with index=1 and limit=2.`, func(t *ftt.Test) {
					got, err := get("A", 1, 2, false)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, got, should.Resemble([]string{"1", "2"}))
				})

				t.Run(`Will fetch B{10, 12, 13} for B.`, func(t *ftt.Test) {
					got, err := get("B", 0, 0, false)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, got, should.Resemble([]string{"10", "12", "13"}))
				})

				t.Run(`Will fetch B{12, 13} when index=11.`, func(t *ftt.Test) {
					got, err := get("B", 11, 0, false)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, got, should.Resemble([]string{"12", "13"}))
				})

				t.Run(`Will fetch {} for INVALID.`, func(t *ftt.Test) {
					got, err := get("INVALID", 0, 0, false)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, got, should.BeEmpty)
				})
			})

			t.Run(`Testing "Get" (keys only)`, func(t *ftt.Test) {
				t.Run(`Can fetch the full row, "A".`, func(t *ftt.Test) {
					got, err := get("A", 0, 0, true)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, got, should.Resemble([]string{"0", "1", "2", "3", "4"}))
				})

				t.Run(`Will fetch A{1, 2, 3, 4} with index=1.`, func(t *ftt.Test) {
					got, err := get("A", 1, 0, true)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, got, should.Resemble([]string{"1", "2", "3", "4"}))
				})

				t.Run(`Will fetch A{1, 2} with index=1 and limit=2.`, func(t *ftt.Test) {
					got, err := get("A", 1, 2, true)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, got, should.Resemble([]string{"1", "2"}))
				})

				t.Run(`Will fetch B{10, 12, 13} for B.`, func(t *ftt.Test) {
					got, err := get("B", 0, 0, true)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, got, should.Resemble([]string{"10", "12", "13"}))
				})

				t.Run(`Will fetch B{12, 13} when index=11.`, func(t *ftt.Test) {
					got, err := get("B", 11, 0, true)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, got, should.Resemble([]string{"12", "13"}))
				})
			})

			t.Run(`Testing "Tail"...`, func(t *ftt.Test) {
				tail := func(path string) (string, error) {
					e, err := s.Tail(c, project, types.StreamPath(path))
					if err != nil {
						return "", err
					}
					return string(e.D), nil
				}

				t.Run(`A tail request for "A" returns A{4}.`, func(t *ftt.Test) {
					got, err := tail("A")
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, got, should.Equal("4"))

					t.Run(`(Cache) A second request also returns A{4}.`, func(t *ftt.Test) {
						got, err := tail("A")
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, got, should.Equal("4"))
					})
				})

				t.Run(`A tail request for "B" returns nothing (no contiguous logs).`, func(t *ftt.Test) {
					_, err := tail("B")
					assert.Loosely(t, err, should.Equal(storage.ErrDoesNotExist))
				})

				t.Run(`A tail request for "C" returns 2.`, func(t *ftt.Test) {
					got, err := tail("C")
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, got, should.Equal("2"))

					t.Run(`(Cache) A second request also returns 2.`, func(t *ftt.Test) {
						got, err := tail("C")
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, got, should.Equal("2"))
					})

					t.Run(`(Cache) After "3" is added, a second request returns 4.`, func(t *ftt.Test) {
						assert.Loosely(t, put("C", 3, "3"), should.BeNil)

						got, err := tail("C")
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, got, should.Equal("4"))
					})
				})

				t.Run(`A tail request for "INVALID" errors NOT FOUND.`, func(t *ftt.Test) {
					_, err := tail("INVALID")
					assert.Loosely(t, err, should.Equal(storage.ErrDoesNotExist))
				})
			})
		})
	})
}
