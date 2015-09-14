// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"fmt"
	"testing"
	"time"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

type qExpect struct {
	q     ds.Query
	inTxn bool

	get  []ds.PropertyMap
	keys []ds.Key
}

type qExStage struct {
	addIdxs []*ds.IndexDefinition
	putEnts []ds.PropertyMap
	delEnts []ds.Key

	expect []qExpect

	extraFns []func(context.Context)
}

type qExTest struct {
	name string
	test []qExStage
}

var stage1Data = []ds.PropertyMap{
	pmap("$key", key("Kind", 1), NEXT,
		"Val", 1, 2, 3, NEXT,
		"Extra", "hello",
	),
	pmap("$key", key("Kind", 2), NEXT,
		"Val", 6, 8, 7, NEXT,
		"When", 27, NEXT,
		"Extra", "zebra",
	),
	pmap("$key", key("Kind", 3), NEXT,
		"Val", 1, 2, 2, 100, NEXT,
		"When", 996688461000000, NEXT,
		"Extra", "waffle",
	),
	pmap("$key", key("Kind", 6), NEXT,
		"Val", 5, NEXT,
		"When", time.Date(2000, time.January, 1, 1, 1, 1, 1, time.UTC), NEXT,
		"Extra", "waffle",
	),
	pmap("$key", key("Child", "seven", key("Kind", 3)), NEXT,
		"Interesting", 28, NEXT,
		"Extra", "hello",
	),
	pmap("$key", key("Unique", 1), NEXT,
		"Derp", 39,
	),
}

var stage2Data = []ds.PropertyMap{
	pmap("$key", key("Kind", 1, key("Kind", 3)), NEXT,
		"Val", 2, 4, 28, NEXT,
		"Extra", "hello", "waffle",
	),
	pmap("$key", key("Kind", 2, key("Kind", 3)), NEXT,
		"Val", 3, 4, NEXT,
		"Extra", "hello", "waffle",
	),
	pmap("$key", key("Kind", 3, key("Kind", 3)), NEXT,
		"Val", 3, 4, 2, 1, NEXT,
		"Extra", "nuts",
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
				{q: nq("Wat").Filter("meep =", 1).Filter("deep =", 2).Order("opt").Order("other"),
					get: []ds.PropertyMap{}},
			},
		},

		{
			putEnts: stage1Data,
			expect: []qExpect{
				{q: nq("Kind"), get: []ds.PropertyMap{}},
				{q: nq("Child").Ancestor(key("Kind", 3)), keys: []ds.Key{
					key("Child", "seven", key("Kind", 3)),
				}},
			},
		},

		{
			putEnts: stage2Data,
			delEnts: []ds.Key{key("Unique", 1)},
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

				{q: nq("Missing").Filter("Id <", 2).Filter("Id >", 2), get: []ds.PropertyMap{}},

				{q: nq("Missing").Filter("Bogus =", 3), get: []ds.PropertyMap{}},

				{q: nq("Kind").Filter("Extra =", "waffle"), get: []ds.PropertyMap{
					stage1Data[2], stage1Data[3],
				}},

				// get ziggy with it
				{q: nq("Kind").Filter("Extra =", "waffle").Filter("Val =", 100), get: []ds.PropertyMap{
					stage1Data[2],
				}},
				{q: nq("Child").Filter("Interesting =", 28).Filter("Extra =", "hello"), get: []ds.PropertyMap{
					stage1Data[4],
				}},

				{q: (nq("Kind").Ancestor(key("Kind", 3)).Order("Val").
					Start(curs("Val", 1, "__key__", key("Kind", 3))).
					End(curs("Val", 90, "__key__", key("Zeta", "woot", key("Kind", 3))))), keys: []ds.Key{},
				},

				{q: nq("Kind").Filter("Val >", 2).Filter("Val <=", 5), get: []ds.PropertyMap{
					stage1Data[0], stage1Data[3],
				}},

				{q: nq("Kind").Filter("Val >", 2).Filter("Val <=", 5).Order("-Val"), get: []ds.PropertyMap{
					stage1Data[3], stage1Data[0],
				}},

				{q: nq("").Filter("__key__ >", key("Kind", 2)), get: []ds.PropertyMap{
					// TODO(riannucci): determine if the real datastore shows metadata
					// during kindless queries. The documentation seems to imply so, but
					// I'd like to be sure.
					pmap("$key", key("__entity_group__", 1, key("Kind", 2)), NEXT,
						"__version__", 1),
					stage1Data[2],
					stage1Data[4],
					// this is 5 because the value is retrieved from HEAD and not from
					// the index snapshot!
					pmap("$key", key("__entity_group__", 1, key("Kind", 3)), NEXT,
						"__version__", 5),
					stage1Data[3],
					pmap("$key", key("__entity_group__", 1, key("Kind", 6)), NEXT,
						"__version__", 1),
					pmap("$key", key("__entity_group__", 1, key("Unique", 1)), NEXT,
						"__version__", 2),
				}},

				{q: (nq("Kind").
					Filter("Val >", 2).Filter("Extra =", "waffle").
					Order("-Val").
					Ancestor(key("Kind", 3))),
					get: []ds.PropertyMap{
						stage1Data[2],
						stage2Data[0],
						stage2Data[1],
					}},

				{q: (nq("Kind").
					Filter("Val >", 2).Filter("Extra =", "waffle").
					Order("-Val").Order("-__key__").
					Ancestor(key("Kind", 3))),
					get: []ds.PropertyMap{
						stage1Data[2],
						stage2Data[0],
						stage2Data[1],
					}},

				{q: (nq("Kind").
					Filter("Val >", 2).Filter("Extra =", "waffle").
					Order("-Val").
					Ancestor(key("Kind", 3)).Project("Val")),
					get: []ds.PropertyMap{
						pmap("$key", key("Kind", 3), NEXT,
							"Val", 100),
						pmap("$key", key("Kind", 1, key("Kind", 3)), NEXT,
							"Val", 28),
						pmap("$key", key("Kind", 1, key("Kind", 3)), NEXT,
							"Val", 4),
						pmap("$key", key("Kind", 2, key("Kind", 3)), NEXT,
							"Val", 4),
						pmap("$key", key("Kind", 2, key("Kind", 3)), NEXT,
							"Val", 3),
					}},

				{q: (nq("Kind").
					Filter("Val >", 2).Filter("Extra =", "waffle").
					Order("-Val").
					Ancestor(key("Kind", 3)).Project("Val").Distinct()),
					get: []ds.PropertyMap{
						pmap("$key", key("Kind", 3), NEXT,
							"Val", 100),
						pmap("$key", key("Kind", 1, key("Kind", 3)), NEXT,
							"Val", 28),
						pmap("$key", key("Kind", 1, key("Kind", 3)), NEXT,
							"Val", 4),
						pmap("$key", key("Kind", 2, key("Kind", 3)), NEXT,
							"Val", 3),
					}},

				// Projecting a complex type (time), gets the index type (int64)
				// instead. Additionally, mixed-types within the same index type are
				// smooshed together in the result.
				{q: nq("Kind").Project("When"), get: []ds.PropertyMap{
					pmap("$key", key("Kind", 2), NEXT,
						"When", 27),
					pmap("$key", key("Kind", 6), NEXT,
						"When", 946688461000000),
					pmap("$key", key("Kind", 3), NEXT,
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

					q := nq("").Filter("__key__ >", key("Kind", 2))

					err := data.Run(q, func(pm ds.PropertyMap, gc ds.CursorCB) bool {
						So(pm, ShouldResemble, pmap(
							"$key", key("__entity_group__", 1, key("Kind", 2)), NEXT,
							"__version__", 1))

						err := error(nil)
						curs, err = gc()
						So(err, ShouldBeNil)
						return false
					})
					So(err, ShouldBeNil)

					err = data.Run(q.Start(curs), func(pm ds.PropertyMap, gc ds.CursorCB) bool {
						So(pm, ShouldResemble, stage1Data[2])
						return false
					})
					So(err, ShouldBeNil)
				},

				func(c context.Context) {
					data := ds.Get(c)
					q := nq("Something").Filter("Does =", 2).Order("Not").Order("Work")
					So(data.Run(q, func(ds.Key, ds.CursorCB) bool {
						return true
					}), ShouldErrLike, "Try adding:\n  C:Something/Does/Not/Work")
				},
			},
		},

		{
			expect: []qExpect{
				// eventual consistency; Unique/1 is deleted at HEAD. Keysonly finds it,
				// but 'normal' doesn't.
				{q: nq("Unique").Filter("__key__ >", key("AKind", 5)).Filter("__key__ <=", key("Zeta", "prime")),
					keys: []ds.Key{key("Unique", 1)},
					get:  []ds.PropertyMap{}},

				{q: nq("Kind").Filter("Val =", 1).Filter("Val =", 3), get: []ds.PropertyMap{
					stage1Data[0], stage2Data[2],
				}},
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

							if expect.keys != nil {
								runner(func(c context.Context) error {
									data := ds.Get(c)
									Convey(fmt.Sprintf("expect %d (keys)", j), func() {
										rslt := []ds.Key(nil)
										So(data.GetAll(expect.q, &rslt), ShouldBeNil)
										So(len(rslt), ShouldEqual, len(expect.keys))
										for i, r := range rslt {
											So(r, ShouldResemble, expect.keys[i])
										}
									})
									return nil
								}, &ds.TransactionOptions{XG: true})
							}

							if expect.get != nil {
								Convey(fmt.Sprintf("expect %d (data)", j), func() {
									runner(func(c context.Context) error {
										rslt := []ds.PropertyMap(nil)
										So(data.GetAll(expect.q, &rslt), ShouldBeNil)
										So(len(rslt), ShouldEqual, len(expect.get))
										for i, r := range rslt {
											So(r, ShouldResemble, expect.get[i])
										}
										return nil
									}, &ds.TransactionOptions{XG: true})
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
}
