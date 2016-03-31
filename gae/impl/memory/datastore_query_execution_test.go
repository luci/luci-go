// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/luci/gae/service/blobstore"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	"golang.org/x/net/context"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

type qExpect struct {
	q     *ds.Query
	inTxn bool

	get   []ds.PropertyMap
	keys  []*ds.Key
	count int
}

type qExStage struct {
	addIdxs []*ds.IndexDefinition
	putEnts []ds.PropertyMap
	delEnts []*ds.Key

	expect []qExpect

	extraFns []func(context.Context)
}

type qExTest struct {
	name string
	test []qExStage
}

var stage1Data = []ds.PropertyMap{
	pmap("$key", key("Kind", 1), Next,
		"Val", 1, 2, 3, Next,
		"Extra", "hello",
	),
	pmap("$key", key("Kind", 2), Next,
		"Val", 6, 8, 7, Next,
		"When", 27, Next,
		"Extra", "zebra",
	),
	pmap("$key", key("Kind", 3), Next,
		"Val", 1, 2, 2, 100, Next,
		"When", 996688461000000, Next,
		"Extra", "waffle",
	),
	pmap("$key", key("Kind", 6), Next,
		"Val", 5, 3, 2, Next,
		"When", time.Date(2000, time.January, 1, 1, 1, 1, 1, time.UTC), Next,
		"Extra", "waffle",
	),
	pmap("$key", key("Kind", 3, "Child", "seven"), Next,
		"Interesting", 28, Next,
		"Extra", "hello",
	),
	pmap("$key", key("Unique", 1), Next,
		"Derp", 39,
	),
}

var stage2Data = []ds.PropertyMap{
	pmap("$key", key("Kind", 3, "Kind", 1), Next,
		"Val", 2, 4, 28, Next,
		"Extra", "hello", "waffle",
	),
	pmap("$key", key("Kind", 3, "Kind", 2), Next,
		"Val", 3, 4, Next,
		"Extra", "hello", "waffle",
	),
	pmap("$key", key("Kind", 3, "Kind", 3), Next,
		"Val", 3, 4, 2, 1, Next,
		"Extra", "nuts",
	),
}

var collapsedData = []ds.PropertyMap{
	// PTTime
	pmap("$key", key("Kind", 1), Next,
		"Date", time.Date(2000, time.January, 1, 1, 1, 1, 1, time.UTC), Next,
	),
	pmap("$key", key("Kind", 2), Next,
		"Date", time.Date(2000, time.March, 1, 1, 1, 1, 1, time.UTC), Next,
	),

	// PTBlobKey
	pmap("$key", key("Kind", 3), Next,
		"Key", blobstore.Key("foo"), Next,
	),
	pmap("$key", key("Kind", 4), Next,
		"Key", blobstore.Key("qux"), Next,
	),

	// PTBytes
	pmap("$key", key("Kind", 5), Next,
		"Val", []byte("ohai"), Next,
	),
	pmap("$key", key("Kind", 6), Next,
		"Val", []byte("uwutm8"), Next,
	),
}

var queryExecutionTests = []qExTest{
	{"basic", []qExStage{
		{
			addIdxs: []*ds.IndexDefinition{
				indx("Unrelated", "-thing", "bob", "-__key__"),
				indx("Wat", "deep", "opt", "other"),
				indx("Wat", "meep", "opt", "other"),
			},
		},

		{
			expect: []qExpect{
				// tests the case where the query has indexes to fulfill it, but there
				// are no actual entities in the datastore.
				{q: nq("Wat").Eq("meep", 1).Eq("deep", 2).Order("opt", "other"),
					get: []ds.PropertyMap{}},
			},
		},

		{
			putEnts: stage1Data,
			expect: []qExpect{
				{q: nq("Kind"), get: []ds.PropertyMap{}},
				{q: nq("Child").Ancestor(key("Kind", 3)), keys: []*ds.Key{
					key("Kind", 3, "Child", "seven"),
				}},
				{q: nq("Child").Ancestor(key("Kind", 3)).EventualConsistency(true), keys: []*ds.Key{}},
				{q: nq("Child").Ancestor(key("Kind", 3)).EventualConsistency(true), keys: []*ds.Key{
					key("Kind", 3, "Child", "seven"),
				}, inTxn: true},
				{q: nq("Child").Ancestor(key("Kind", 3)), keys: []*ds.Key{
					key("Kind", 3, "Child", "seven"),
				}, inTxn: true},
			},
		},

		{
			putEnts: stage2Data,
			delEnts: []*ds.Key{key("Unique", 1)},
			addIdxs: []*ds.IndexDefinition{
				indx("Kind!", "-Extra", "-Val"),
				indx("Kind!", "-Extra", "-Val", "-__key__"),
				indx("Kind!", "Bogus", "Extra", "-Val"),
			},
			expect: []qExpect{
				{q: nq("Kind"), get: stage1Data[:4]},

				{q: nq("Kind").Offset(2).Limit(1), get: []ds.PropertyMap{
					stage1Data[2],
				}},

				{q: nq("Missing"), get: []ds.PropertyMap{}},

				{q: nq("Missing").Eq("Bogus", 3), get: []ds.PropertyMap{}},

				{q: nq("Kind").Eq("Extra", "waffle"), get: []ds.PropertyMap{
					stage1Data[2], stage1Data[3],
				}},

				// get ziggy with it
				{q: nq("Kind").Eq("Extra", "waffle").Eq("Val", 100), get: []ds.PropertyMap{
					stage1Data[2],
				}},

				{q: nq("Child").Eq("Interesting", 28).Eq("Extra", "hello"), get: []ds.PropertyMap{
					stage1Data[4],
				}},

				{q: nq("Kind").Eq("Val", 2, 3), get: []ds.PropertyMap{
					stage1Data[0],
					stage1Data[3],
				}},

				// note the kind :)
				{q: (nq("Kind").Ancestor(key("Kind", 3)).
					Start(curs("__key__", key("Kind", 3))).
					End(curs("__key__", key("Kind", 3, "Zeta", "woot")))),
					keys: []*ds.Key{
						key("Kind", 3),
						key("Kind", 3, "Kind", 1),
						key("Kind", 3, "Kind", 2),
						key("Kind", 3, "Kind", 3),
					},
				},

				{q: (nq("").Ancestor(key("Kind", 3)).
					Start(curs("__key__", key("Kind", 3))).
					End(curs("__key__", key("Kind", 3, "Zeta", "woot")))),
					keys: []*ds.Key{
						key("Kind", 3),
						key("Kind", 3, "Child", "seven"),
						key("Kind", 3, "Kind", 1),
						key("Kind", 3, "Kind", 2),
						key("Kind", 3, "Kind", 3),
					},
				},

				{q: (nq("Kind").Ancestor(key("Kind", 3)).
					Start(curs("__key__", key("Kind", 3))).
					End(curs("__key__", key("Kind", 3, "Zeta", "woot")))),
					keys: []*ds.Key{
						key("Kind", 3),
						key("Kind", 3, "Kind", 1),
						key("Kind", 3, "Kind", 2),
						key("Kind", 3, "Kind", 3),
					},
					inTxn: true},

				{q: nq("Kind").Ancestor(key("Kind", 3)).Eq("Val", 3, 4),
					keys: []*ds.Key{
						key("Kind", 3, "Kind", 2),
						key("Kind", 3, "Kind", 3),
					},
					get: []ds.PropertyMap{
						stage2Data[1],
						stage2Data[2],
					},
				},

				{q: nq("Kind").Gt("Val", 2).Lte("Val", 5), get: []ds.PropertyMap{
					stage1Data[0], stage1Data[3],
				}},

				{q: nq("Kind").Gt("Val", 2).Lte("Val", 5).Order("-Val"), get: []ds.PropertyMap{
					stage1Data[3], stage1Data[0],
				}},

				{q: nq("").Gt("__key__", key("Kind", 2)),
					// count counts from the index with KeysOnly and so counts the deleted
					// entity Unique/1.
					count: 8,
					get: []ds.PropertyMap{
						// TODO(riannucci): determine if the real datastore shows metadata
						// during kindless queries. The documentation seems to imply so, but
						// I'd like to be sure.
						pmap("$key", key("Kind", 2, "__entity_group__", 1), Next,
							"__version__", 1),
						stage1Data[2],
						stage1Data[4],
						// this is 5 because the value is retrieved from HEAD and not from
						// the index snapshot!
						pmap("$key", key("Kind", 3, "__entity_group__", 1), Next,
							"__version__", 5),
						stage1Data[3],
						pmap("$key", key("Kind", 6, "__entity_group__", 1), Next,
							"__version__", 1),
						pmap("$key", key("Unique", 1, "__entity_group__", 1), Next,
							"__version__", 2),
					}},

				{q: (nq("Kind").
					Gt("Val", 2).Eq("Extra", "waffle").
					Order("-Val").
					Ancestor(key("Kind", 3))),
					get: []ds.PropertyMap{
						stage1Data[2],
						stage2Data[0],
						stage2Data[1],
					}},

				{q: (nq("Kind").
					Gt("Val", 2).Eq("Extra", "waffle").
					Order("-Val").
					Ancestor(key("Kind", 3))),
					get: []ds.PropertyMap{
						stage1Data[2],
						stage2Data[0],
						stage2Data[1],
					}, inTxn: true},

				{q: (nq("Kind").
					Gt("Val", 2).Eq("Extra", "waffle").
					Order("-Val", "-__key__").
					Ancestor(key("Kind", 3))),
					get: []ds.PropertyMap{
						stage1Data[2],
						stage2Data[0],
						stage2Data[1],
					}},

				{q: (nq("Kind").
					Gt("Val", 2).Eq("Extra", "waffle").
					Order("-Val").
					Ancestor(key("Kind", 3)).Project("Val")),
					get: []ds.PropertyMap{
						pmap("$key", key("Kind", 3), Next,
							"Val", 100),
						pmap("$key", key("Kind", 3, "Kind", 1), Next,
							"Val", 28),
						pmap("$key", key("Kind", 3, "Kind", 1), Next,
							"Val", 4),
						pmap("$key", key("Kind", 3, "Kind", 2), Next,
							"Val", 4),
						pmap("$key", key("Kind", 3, "Kind", 2), Next,
							"Val", 3),
					}},

				{q: (nq("Kind").
					Gt("Val", 2).Eq("Extra", "waffle").
					Order("-Val").
					Ancestor(key("Kind", 3)).Project("Val").Distinct(true)),
					get: []ds.PropertyMap{
						pmap("$key", key("Kind", 3), Next,
							"Val", 100),
						pmap("$key", key("Kind", 3, "Kind", 1), Next,
							"Val", 28),
						pmap("$key", key("Kind", 3, "Kind", 1), Next,
							"Val", 4),
						pmap("$key", key("Kind", 3, "Kind", 2), Next,
							"Val", 3),
					}},

				// Projecting a complex type (time), gets the index type (int64)
				// instead. Additionally, mixed-types within the same index type are
				// smooshed together in the result.
				{q: nq("Kind").Project("When"), get: []ds.PropertyMap{
					pmap("$key", key("Kind", 2), Next,
						"When", 27),
					pmap("$key", key("Kind", 6), Next,
						"When", 946688461000000),
					pmap("$key", key("Kind", 3), Next,
						"When", 996688461000000),
				}},

				// Original (complex) types are retained when getting the full value.
				{q: nq("Kind").Order("When"), get: []ds.PropertyMap{
					stage1Data[1],
					stage1Data[3],
					stage1Data[2],
				}},
			},

			extraFns: []func(context.Context){
				func(c context.Context) {
					data := ds.Get(c)
					curs := ds.Cursor(nil)

					q := nq("").Gt("__key__", key("Kind", 2))

					err := data.Run(q, func(pm ds.PropertyMap, gc ds.CursorCB) error {
						So(pm, ShouldResemble, pmap(
							"$key", key("Kind", 2, "__entity_group__", 1), Next,
							"__version__", 1))

						err := error(nil)
						curs, err = gc()
						So(err, ShouldBeNil)
						return ds.Stop
					})
					So(err, shouldBeSuccessful)

					err = data.Run(q.Start(curs), func(pm ds.PropertyMap) error {
						So(pm, ShouldResemble, stage1Data[2])
						return ds.Stop
					})
					So(err, shouldBeSuccessful)
				},

				func(c context.Context) {
					data := ds.Get(c)
					q := nq("Something").Eq("Does", 2).Order("Not", "-Work")
					So(data.Run(q, func(ds.Key) {}), ShouldErrLike, strings.Join([]string{
						"Consider adding:",
						"- kind: Something",
						"  properties:",
						"  - name: Does",
						"  - name: Not",
						"  - name: Work",
						"    direction: desc",
					}, "\n"))
				},

				func(c context.Context) {
					data := ds.Get(c)
					q := nq("Something").Ancestor(key("Kind", 3)).Order("Val")
					So(data.Run(q, func(ds.Key) {}), ShouldErrLike, strings.Join([]string{
						"Consider adding:",
						"- kind: Something",
						"  ancestor: yes",
						"  properties:",
						"  - name: Val",
					}, "\n"))
				},
			},
		},

		{
			expect: []qExpect{
				// eventual consistency; Unique/1 is deleted at HEAD. Keysonly finds it,
				// but 'normal' doesn't.
				{q: nq("Unique").Gt("__key__", key("AKind", 5)).Lte("__key__", key("Zeta", "prime")),
					keys: []*ds.Key{key("Unique", 1)},
					get:  []ds.PropertyMap{}},

				{q: nq("Kind").Eq("Val", 1, 3), get: []ds.PropertyMap{
					stage1Data[0], stage2Data[2],
				}},
			},
		},
	}},
	{"collapsed types", []qExStage{
		{
			putEnts: collapsedData,
		},
		{
			expect: []qExpect{
				// PTTime
				{
					q: nq("Kind").Lte("Date", time.Date(2000, time.February, 1, 1, 1, 1, 1, time.UTC)),
					get: []ds.PropertyMap{
						collapsedData[0],
					},
				},
				{
					q: nq("Kind").Eq("Date", time.Date(2000, time.March, 1, 1, 1, 1, 1, time.UTC)),
					get: []ds.PropertyMap{
						collapsedData[1],
					},
				},

				// PTBlobKey
				{
					q: nq("Kind").Lte("Key", blobstore.Key("foo")),
					get: []ds.PropertyMap{
						collapsedData[2],
					},
				},
				{
					q: nq("Kind").Eq("Key", blobstore.Key("qux")),
					get: []ds.PropertyMap{
						collapsedData[3],
					},
				},

				// PTBytes
				{
					q: nq("Kind").Lte("Val", []byte("ohai")),
					get: []ds.PropertyMap{
						collapsedData[4],
					},
				},
				{
					q: nq("Kind").Eq("Val", []byte("uwutm8")),
					get: []ds.PropertyMap{
						collapsedData[5],
					},
				},
			},
		},
	}},
}

func TestQueryExecution(t *testing.T) {
	t.Parallel()

	Convey("Test query execution", t, func() {
		c, err := info.Get(Use(context.Background())).Namespace("ns")
		if err != nil {
			panic(err)
		}

		So(info.Get(c).FullyQualifiedAppID(), ShouldEqual, "dev~app")
		So(info.Get(c).GetNamespace(), ShouldEqual, "ns")

		data := ds.Get(c)
		testing := data.Testable()

		for _, tc := range queryExecutionTests {
			Convey(tc.name, func() {
				for i, stage := range tc.test {
					// outside of Convey, since these must always happen
					testing.CatchupIndexes()

					testing.AddIndexes(stage.addIdxs...)
					if err := data.PutMulti(stage.putEnts); err != nil {
						// prevent Convey from thinking this assertion should show up in
						// every test loop.
						panic(err)
					}

					if err := data.DeleteMulti(stage.delEnts); err != nil {
						panic(err)
					}

					Convey(fmt.Sprintf("stage %d", i), func() {
						for j, expect := range stage.expect {
							runner := func(f func(ic context.Context) error, _ *ds.TransactionOptions) error {
								return f(c)
							}
							if expect.inTxn {
								runner = data.RunInTransaction
							}

							if expect.count == 0 {
								if len(expect.keys) > 0 {
									expect.count = len(expect.keys)
								} else {
									expect.count = len(expect.get)
								}
							}

							if expect.keys != nil {
								Convey(fmt.Sprintf("expect %d (keys)", j), func() {
									err := runner(func(c context.Context) error {
										data := ds.Get(c)
										count, err := data.Count(expect.q)
										So(err, shouldBeSuccessful)
										So(count, ShouldEqual, expect.count)

										rslt := []*ds.Key(nil)
										So(data.GetAll(expect.q, &rslt), shouldBeSuccessful)
										So(len(rslt), ShouldEqual, len(expect.keys))
										for i, r := range rslt {
											So(r, ShouldResemble, expect.keys[i])
										}
										return nil
									}, &ds.TransactionOptions{XG: true})
									So(err, shouldBeSuccessful)
								})
							}

							if expect.get != nil {
								Convey(fmt.Sprintf("expect %d (data)", j), func() {
									err := runner(func(c context.Context) error {
										data := ds.Get(c)
										count, err := data.Count(expect.q)
										So(err, shouldBeSuccessful)
										So(count, ShouldEqual, expect.count)

										rslt := []ds.PropertyMap(nil)
										So(data.GetAll(expect.q, &rslt), shouldBeSuccessful)
										So(len(rslt), ShouldEqual, len(expect.get))
										for i, r := range rslt {
											So(r, ShouldResemble, expect.get[i])
										}
										return nil
									}, &ds.TransactionOptions{XG: true})
									So(err, shouldBeSuccessful)
								})
							}
						}

						for j, fn := range stage.extraFns {
							Convey(fmt.Sprintf("extraFn %d", j), func() {
								fn(c)
							})
						}
					})
				}
			})
		}
	})

	Convey("Test AutoIndex", t, func() {
		c, err := info.Get(Use(context.Background())).Namespace("ns")
		if err != nil {
			panic(err)
		}

		data := ds.Get(c)
		testing := data.Testable()
		testing.Consistent(true)

		So(data.Put(pmap("$key", key("Kind", 1), Next,
			"Val", 1, 2, 3, Next,
			"Extra", "hello",
		)), shouldBeSuccessful)

		So(data.Put(pmap("$key", key("Kind", 2), Next,
			"Val", 2, 3, 9, Next,
			"Extra", "ace", "hello", "there",
		)), shouldBeSuccessful)

		q := nq("Kind").Gt("Val", 2).Order("Val", "Extra")

		count, err := data.Count(q)
		So(err, ShouldErrLike, "Insufficient indexes")

		testing.AutoIndex(true)

		count, err = data.Count(q)
		So(err, shouldBeSuccessful)
		So(count, ShouldEqual, 2)
	})
}

func shouldBeSuccessful(actual interface{}, expected ...interface{}) string {
	if len(expected) != 0 {
		return "no expected values permitted"
	}
	if actual == nil {
		return ""
	}

	v, ok := actual.(error)
	if !ok {
		return fmt.Sprintf("type of 'actual' must be error, not %T", actual)
	}
	if v == nil || v == ds.Stop {
		return ""
	}
	return fmt.Sprintf("expected success value, not %v", v)
}
