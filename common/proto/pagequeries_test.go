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

package proto

import (
	"context"
	"testing"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/common/proto/examples"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestPageQueries(t *testing.T) {
	t.Parallel()

	Convey("PageQuery", t, func() {
		type Record struct {
			_kind string `gae:"$kind,kind"`
			ID    string `gae:"$id"`
		}

		c := memory.Use(context.Background())
		datastore.GetTestable(c).AutoIndex(true)
		datastore.GetTestable(c).Consistent(true)
		rsp := &examples.ListResponse{}
		q := datastore.NewQuery("kind")

		Convey("invalid", func() {
			Convey("function", func() {
				Convey("nil", func() {
					So(PageQuery(c, 0, "", rsp, q, nil), ShouldErrLike, "callback must be a function")
				})

				Convey("no inputs", func() {
					f := func() error {
						return nil
					}
					So(PageQuery(c, 0, "", rsp, q, f), ShouldErrLike, "callback function must accept one argument")
				})

				Convey("many inputs", func() {
					f := func(interface{}, datastore.CursorCB) error {
						return nil
					}
					So(PageQuery(c, 0, "", rsp, q, f), ShouldErrLike, "callback function must accept one argument")
				})

				Convey("no outputs", func() {
					f := func(interface{}) {
					}
					So(PageQuery(c, 0, "", rsp, q, f), ShouldErrLike, "callback function must return one value")
				})

				Convey("many outputs", func() {
					f := func(interface{}) (interface{}, error) {
						return nil, nil
					}
					So(PageQuery(c, 0, "", rsp, q, f), ShouldErrLike, "callback function must return one value")
				})
			})

			Convey("token", func() {
				f := func(interface{}) error {
					return nil
				}
				So(PageQuery(c, 0, "tok", rsp, q, f), ShouldErrLike, "invalid page token")
			})
		})

		Convey("valid", func() {
			Convey("callback", func() {
				Convey("error", func() {
					f := func(r *Record) error {
						return errors.New("error")
					}
					So(datastore.Put(c, &Record{ID: "id"}), ShouldBeNil)

					So(PageQuery(c, 0, "", rsp, q, f), ShouldErrLike, "error")
				})

				Convey("stop", func() {
					f := func(*Record) error {
						return datastore.Stop
					}

					Convey("first", func() {
						So(datastore.Put(c, &Record{ID: "id1"}), ShouldBeNil)
						So(datastore.Put(c, &Record{ID: "id2"}), ShouldBeNil)

						Convey("limit", func() {
							Convey("greater", func() {
								So(PageQuery(c, 10, "", rsp, q, f), ShouldBeNil)
								So(rsp.NextPageToken, ShouldNotBeEmpty)

								tok := rsp.NextPageToken
								rsp.NextPageToken = ""
								So(PageQuery(c, 10, tok, rsp, q, f), ShouldBeNil)
								So(rsp.NextPageToken, ShouldBeEmpty)
							})

							Convey("equal", func() {
								So(PageQuery(c, 1, "", rsp, q, f), ShouldBeNil)
								So(rsp.NextPageToken, ShouldNotBeEmpty)

								tok := rsp.NextPageToken
								rsp.NextPageToken = ""
								So(PageQuery(c, 1, tok, rsp, q, f), ShouldBeNil)
								So(rsp.NextPageToken, ShouldBeEmpty)
							})
						})

						Convey("no limit", func() {
							So(PageQuery(c, 0, "", rsp, q, f), ShouldBeNil)
							So(rsp.NextPageToken, ShouldNotBeEmpty)

							tok := rsp.NextPageToken
							rsp.NextPageToken = ""
							So(PageQuery(c, 0, tok, rsp, q, f), ShouldBeNil)
							So(rsp.NextPageToken, ShouldBeEmpty)
						})
					})

					Convey("intermediate", func() {
						i := 0
						f = func(*Record) error {
							i++
							if i == 2 {
								return datastore.Stop
							}
							return nil
						}
						So(datastore.Put(c, &Record{ID: "id1"}), ShouldBeNil)
						So(datastore.Put(c, &Record{ID: "id2"}), ShouldBeNil)
						So(datastore.Put(c, &Record{ID: "id3"}), ShouldBeNil)

						Convey("limit", func() {
							Convey("greater", func() {
								So(PageQuery(c, 10, "", rsp, q, f), ShouldBeNil)
								So(rsp.NextPageToken, ShouldNotBeEmpty)

								tok := rsp.NextPageToken
								rsp.NextPageToken = ""
								So(PageQuery(c, 10, tok, rsp, q, f), ShouldBeNil)
								So(rsp.NextPageToken, ShouldBeEmpty)
							})

							Convey("equal", func() {
								So(PageQuery(c, 2, "", rsp, q, f), ShouldBeNil)
								So(rsp.NextPageToken, ShouldNotBeEmpty)

								tok := rsp.NextPageToken
								rsp.NextPageToken = ""
								So(PageQuery(c, 2, tok, rsp, q, f), ShouldBeNil)
								So(rsp.NextPageToken, ShouldBeEmpty)
							})
						})

						Convey("no limit", func() {
							So(PageQuery(c, 0, "", rsp, q, f), ShouldBeNil)
							So(rsp.NextPageToken, ShouldNotBeEmpty)

							tok := rsp.NextPageToken
							rsp.NextPageToken = ""
							So(PageQuery(c, 0, tok, rsp, q, f), ShouldBeNil)
							So(rsp.NextPageToken, ShouldBeEmpty)
						})
					})

					Convey("last", func() {
						So(datastore.Put(c, &Record{ID: "id"}), ShouldBeNil)

						Convey("limit", func() {
							Convey("greater", func() {
								So(PageQuery(c, 10, "", rsp, q, f), ShouldBeNil)
								So(rsp.NextPageToken, ShouldBeEmpty)
							})

							Convey("equal", func() {
								So(PageQuery(c, 1, "", rsp, q, f), ShouldBeNil)
								So(rsp.NextPageToken, ShouldBeEmpty)
							})
						})

						Convey("no limit", func() {
							So(PageQuery(c, 0, "", rsp, q, f), ShouldBeNil)
							So(rsp.NextPageToken, ShouldBeEmpty)
						})
					})
				})

				Convey("ok", func() {
					f := func(*Record) error {
						return nil
					}
					So(datastore.Put(c, &Record{ID: "id"}), ShouldBeNil)

					Convey("limit", func() {
						So(PageQuery(c, 10, "", rsp, q, f), ShouldBeNil)
					})

					Convey("no limit", func() {
						So(PageQuery(c, 0, "", rsp, q, f), ShouldBeNil)
					})
				})
			})

			Convey("query", func() {
				rsp.Records = make([]string, 0)
				f := func(r *Record) error {
					rsp.Records = append(rsp.Records, r.ID)
					return nil
				}

				Convey("limit", func() {
					Convey("none", func() {
						So(PageQuery(c, 2, "", rsp, q, f), ShouldBeNil)
						So(rsp.Records, ShouldBeEmpty)
					})

					Convey("one", func() {
						So(datastore.Put(c, &Record{ID: "id"}), ShouldBeNil)

						So(PageQuery(c, 2, "", rsp, q, f), ShouldBeNil)
						So(rsp.Records, ShouldResemble, []string{"id"})
						So(rsp.NextPageToken, ShouldBeEmpty)
					})

					Convey("many", func() {
						So(datastore.Put(c, &Record{ID: "id1"}), ShouldBeNil)
						So(datastore.Put(c, &Record{ID: "id2"}), ShouldBeNil)
						So(datastore.Put(c, &Record{ID: "id3"}), ShouldBeNil)

						So(PageQuery(c, 2, "", rsp, q, f), ShouldBeNil)
						So(rsp.Records, ShouldResemble, []string{"id1", "id2"})
						So(rsp.NextPageToken, ShouldNotBeEmpty)

						tok := rsp.NextPageToken
						rsp.NextPageToken = ""
						rsp.Records = make([]string, 0)
						So(PageQuery(c, 2, tok, rsp, q, f), ShouldBeNil)
						So(rsp.Records, ShouldResemble, []string{"id3"})
						So(rsp.NextPageToken, ShouldBeEmpty)
					})
				})

				Convey("no limit", func() {
					Convey("none", func() {
						So(PageQuery(c, 0, "", rsp, q, f), ShouldBeNil)
						So(rsp.Records, ShouldBeEmpty)
					})

					Convey("one", func() {
						So(datastore.Put(c, &Record{ID: "id"}), ShouldBeNil)

						So(PageQuery(c, 0, "", rsp, q, f), ShouldBeNil)
						So(rsp.Records, ShouldResemble, []string{"id"})
					})

					Convey("many", func() {
						So(datastore.Put(c, &Record{ID: "id1"}), ShouldBeNil)
						So(datastore.Put(c, &Record{ID: "id2"}), ShouldBeNil)
						So(datastore.Put(c, &Record{ID: "id3"}), ShouldBeNil)

						So(PageQuery(c, 0, "", rsp, q, f), ShouldBeNil)
						So(rsp.Records, ShouldResemble, []string{"id1", "id2", "id3"})
					})

					Convey("error", func() {
						rsp.Records = make([]string, 0)
						f := func(r *Record) error {
							return errors.Reason("error").Err()
						}
						So(datastore.Put(c, &Record{ID: "id1"}), ShouldBeNil)
						So(datastore.Put(c, &Record{ID: "id2"}), ShouldBeNil)
						So(datastore.Put(c, &Record{ID: "id3"}), ShouldBeNil)

						So(PageQuery(c, 0, "", rsp, q, f), ShouldErrLike, "error")
						So(rsp.Records, ShouldBeEmpty)
						So(rsp.NextPageToken, ShouldBeEmpty)
					})
				})
			})
		})
	})
}
