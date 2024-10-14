// Copyright 2019 The LUCI Authors.
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

package paged

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/common/proto/examples"
)

func TestQueries(t *testing.T) {
	t.Parallel()

	ftt.Run("Query", t, func(t *ftt.Test) {
		type Record struct {
			_kind string `gae:"$kind,kind"`
			ID    string `gae:"$id"`
		}

		c := memory.Use(context.Background())
		datastore.GetTestable(c).AutoIndex(true)
		datastore.GetTestable(c).Consistent(true)
		rsp := &examples.ListResponse{}
		q := datastore.NewQuery("kind")

		t.Run("invalid", func(t *ftt.Test) {
			t.Run("function", func(t *ftt.Test) {
				t.Run("nil", func(t *ftt.Test) {
					assert.Loosely(t, Query(c, 0, "", rsp, q, nil), should.ErrLike("callback must be a function"))
				})

				t.Run("no inputs", func(t *ftt.Test) {
					f := func() error {
						return nil
					}
					assert.Loosely(t, Query(c, 0, "", rsp, q, f), should.ErrLike("callback function must accept one argument"))
				})

				t.Run("many inputs", func(t *ftt.Test) {
					f := func(any, datastore.CursorCB) error {
						return nil
					}
					assert.Loosely(t, Query(c, 0, "", rsp, q, f), should.ErrLike("callback function must accept one argument"))
				})

				t.Run("no outputs", func(t *ftt.Test) {
					f := func(any) {
					}
					assert.Loosely(t, Query(c, 0, "", rsp, q, f), should.ErrLike("callback function must return one value"))
				})

				t.Run("many outputs", func(t *ftt.Test) {
					f := func(any) (any, error) {
						return nil, nil
					}
					assert.Loosely(t, Query(c, 0, "", rsp, q, f), should.ErrLike("callback function must return one value"))
				})
			})

			t.Run("token", func(t *ftt.Test) {
				f := func(any) error {
					return nil
				}
				assert.Loosely(t, Query(c, 0, "tok", rsp, q, f), should.ErrLike("invalid page token"))
			})
		})

		t.Run("valid", func(t *ftt.Test) {
			t.Run("callback", func(t *ftt.Test) {
				t.Run("error", func(t *ftt.Test) {
					f := func(r *Record) error {
						return errors.New("error")
					}
					assert.Loosely(t, datastore.Put(c, &Record{ID: "id"}), should.BeNil)

					assert.Loosely(t, Query(c, 0, "", rsp, q, f), should.ErrLike("error"))
				})

				t.Run("stop", func(t *ftt.Test) {
					f := func(*Record) error {
						return datastore.Stop
					}

					t.Run("first", func(t *ftt.Test) {
						assert.Loosely(t, datastore.Put(c, &Record{ID: "id1"}), should.BeNil)
						assert.Loosely(t, datastore.Put(c, &Record{ID: "id2"}), should.BeNil)

						t.Run("limit", func(t *ftt.Test) {
							t.Run("greater", func(t *ftt.Test) {
								assert.Loosely(t, Query(c, 10, "", rsp, q, f), should.BeNil)
								assert.Loosely(t, rsp.NextPageToken, should.NotBeEmpty)

								tok := rsp.NextPageToken
								rsp.NextPageToken = ""
								assert.Loosely(t, Query(c, 10, tok, rsp, q, f), should.BeNil)
								assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
							})

							t.Run("equal", func(t *ftt.Test) {
								assert.Loosely(t, Query(c, 1, "", rsp, q, f), should.BeNil)
								assert.Loosely(t, rsp.NextPageToken, should.NotBeEmpty)

								tok := rsp.NextPageToken
								rsp.NextPageToken = ""
								assert.Loosely(t, Query(c, 1, tok, rsp, q, f), should.BeNil)
								assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
							})
						})

						t.Run("no limit", func(t *ftt.Test) {
							assert.Loosely(t, Query(c, 0, "", rsp, q, f), should.BeNil)
							assert.Loosely(t, rsp.NextPageToken, should.NotBeEmpty)

							tok := rsp.NextPageToken
							rsp.NextPageToken = ""
							assert.Loosely(t, Query(c, 0, tok, rsp, q, f), should.BeNil)
							assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
						})
					})

					t.Run("intermediate", func(t *ftt.Test) {
						i := 0
						f = func(*Record) error {
							i++
							if i == 2 {
								return datastore.Stop
							}
							return nil
						}
						assert.Loosely(t, datastore.Put(c, &Record{ID: "id1"}), should.BeNil)
						assert.Loosely(t, datastore.Put(c, &Record{ID: "id2"}), should.BeNil)
						assert.Loosely(t, datastore.Put(c, &Record{ID: "id3"}), should.BeNil)

						t.Run("limit", func(t *ftt.Test) {
							t.Run("greater", func(t *ftt.Test) {
								assert.Loosely(t, Query(c, 10, "", rsp, q, f), should.BeNil)
								assert.Loosely(t, rsp.NextPageToken, should.NotBeEmpty)

								tok := rsp.NextPageToken
								rsp.NextPageToken = ""
								assert.Loosely(t, Query(c, 10, tok, rsp, q, f), should.BeNil)
								assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
							})

							t.Run("equal", func(t *ftt.Test) {
								assert.Loosely(t, Query(c, 2, "", rsp, q, f), should.BeNil)
								assert.Loosely(t, rsp.NextPageToken, should.NotBeEmpty)

								tok := rsp.NextPageToken
								rsp.NextPageToken = ""
								assert.Loosely(t, Query(c, 2, tok, rsp, q, f), should.BeNil)
								assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
							})
						})

						t.Run("no limit", func(t *ftt.Test) {
							assert.Loosely(t, Query(c, 0, "", rsp, q, f), should.BeNil)
							assert.Loosely(t, rsp.NextPageToken, should.NotBeEmpty)

							tok := rsp.NextPageToken
							rsp.NextPageToken = ""
							assert.Loosely(t, Query(c, 0, tok, rsp, q, f), should.BeNil)
							assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
						})
					})

					t.Run("last", func(t *ftt.Test) {
						assert.Loosely(t, datastore.Put(c, &Record{ID: "id"}), should.BeNil)

						t.Run("limit", func(t *ftt.Test) {
							t.Run("greater", func(t *ftt.Test) {
								assert.Loosely(t, Query(c, 10, "", rsp, q, f), should.BeNil)
								assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
							})

							t.Run("equal", func(t *ftt.Test) {
								assert.Loosely(t, Query(c, 1, "", rsp, q, f), should.BeNil)
								assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
							})
						})

						t.Run("no limit", func(t *ftt.Test) {
							assert.Loosely(t, Query(c, 0, "", rsp, q, f), should.BeNil)
							assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
						})
					})
				})

				t.Run("ok", func(t *ftt.Test) {
					f := func(*Record) error {
						return nil
					}
					assert.Loosely(t, datastore.Put(c, &Record{ID: "id"}), should.BeNil)

					t.Run("limit", func(t *ftt.Test) {
						assert.Loosely(t, Query(c, 10, "", rsp, q, f), should.BeNil)
					})

					t.Run("no limit", func(t *ftt.Test) {
						assert.Loosely(t, Query(c, 0, "", rsp, q, f), should.BeNil)
					})
				})
			})

			t.Run("query", func(t *ftt.Test) {
				rsp.Records = make([]string, 0)
				f := func(r *Record) error {
					rsp.Records = append(rsp.Records, r.ID)
					return nil
				}

				t.Run("limit", func(t *ftt.Test) {
					t.Run("none", func(t *ftt.Test) {
						assert.Loosely(t, Query(c, 2, "", rsp, q, f), should.BeNil)
						assert.Loosely(t, rsp.Records, should.BeEmpty)
					})

					t.Run("one", func(t *ftt.Test) {
						assert.Loosely(t, datastore.Put(c, &Record{ID: "id"}), should.BeNil)

						assert.Loosely(t, Query(c, 2, "", rsp, q, f), should.BeNil)
						assert.Loosely(t, rsp.Records, should.Resemble([]string{"id"}))
						assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
					})

					t.Run("many", func(t *ftt.Test) {
						assert.Loosely(t, datastore.Put(c, &Record{ID: "id1"}), should.BeNil)
						assert.Loosely(t, datastore.Put(c, &Record{ID: "id2"}), should.BeNil)
						assert.Loosely(t, datastore.Put(c, &Record{ID: "id3"}), should.BeNil)

						assert.Loosely(t, Query(c, 2, "", rsp, q, f), should.BeNil)
						assert.Loosely(t, rsp.Records, should.Resemble([]string{"id1", "id2"}))
						assert.Loosely(t, rsp.NextPageToken, should.NotBeEmpty)

						tok := rsp.NextPageToken
						rsp.NextPageToken = ""
						rsp.Records = make([]string, 0)
						assert.Loosely(t, Query(c, 2, tok, rsp, q, f), should.BeNil)
						assert.Loosely(t, rsp.Records, should.Resemble([]string{"id3"}))
						assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
					})
				})

				t.Run("no limit", func(t *ftt.Test) {
					t.Run("none", func(t *ftt.Test) {
						assert.Loosely(t, Query(c, 0, "", rsp, q, f), should.BeNil)
						assert.Loosely(t, rsp.Records, should.BeEmpty)
					})

					t.Run("one", func(t *ftt.Test) {
						assert.Loosely(t, datastore.Put(c, &Record{ID: "id"}), should.BeNil)

						assert.Loosely(t, Query(c, 0, "", rsp, q, f), should.BeNil)
						assert.Loosely(t, rsp.Records, should.Resemble([]string{"id"}))
					})

					t.Run("many", func(t *ftt.Test) {
						assert.Loosely(t, datastore.Put(c, &Record{ID: "id1"}), should.BeNil)
						assert.Loosely(t, datastore.Put(c, &Record{ID: "id2"}), should.BeNil)
						assert.Loosely(t, datastore.Put(c, &Record{ID: "id3"}), should.BeNil)

						assert.Loosely(t, Query(c, 0, "", rsp, q, f), should.BeNil)
						assert.Loosely(t, rsp.Records, should.Resemble([]string{"id1", "id2", "id3"}))
					})

					t.Run("error", func(t *ftt.Test) {
						rsp.Records = make([]string, 0)
						f := func(r *Record) error {
							return errors.Reason("error").Err()
						}
						assert.Loosely(t, datastore.Put(c, &Record{ID: "id1"}), should.BeNil)
						assert.Loosely(t, datastore.Put(c, &Record{ID: "id2"}), should.BeNil)
						assert.Loosely(t, datastore.Put(c, &Record{ID: "id3"}), should.BeNil)

						assert.Loosely(t, Query(c, 0, "", rsp, q, f), should.ErrLike("error"))
						assert.Loosely(t, rsp.Records, should.BeEmpty)
						assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
					})
				})
			})
		})
	})
}
