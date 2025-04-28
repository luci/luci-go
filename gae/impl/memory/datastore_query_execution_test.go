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
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/failure"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/gae/service/blobstore"
	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
)

func mkKey(appID, namespace string, elems ...any) *ds.Key {
	return ds.MkKeyContext(appID, namespace).MakeKey(elems...)
}

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

	extraFns []func(context.Context, *testing.T)
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
	pmap("$key", key("A", 1), Next,
		"Inner", pmap("$key", key("Inner", 2), Next,
			"Prop", 1,
		),
	),
	pmap("$key", key("A", 2), Next,
		"Inner", pmap("Prop", 2),
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
	pmap("$key", mkKey("dev~app", "", "Kind", "id")),
	pmap("$key", mkKey("dev~app", "bob", "Kind", "id")),
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
				{q: nq("Child").FirestoreMode(true), inTxn: true},
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
				{q: nq("__namespace__"), get: []ds.PropertyMap{
					pmap("$key", mkKey("dev~app", "", "__namespace__", "ns")),
				}},
				{q: nq("__namespace__").Offset(1), get: []ds.PropertyMap{}},
				{q: nq("__namespace__").Offset(1).Limit(1), get: []ds.PropertyMap{}},
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
				{q: nq("Kind").In("Extra", "waffle"), get: []ds.PropertyMap{
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
					count: 4,
					get: []ds.PropertyMap{
						stage1Data[2],
						stage1Data[4],
						stage1Data[3],
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
				{q: nq("__namespace__"), get: []ds.PropertyMap{
					pmap("$key", mkKey("dev~app", "", "__namespace__", 1)),
					pmap("$key", mkKey("dev~app", "", "__namespace__", "bob")),
					pmap("$key", mkKey("dev~app", "", "__namespace__", "ns")),
				}},
				{q: nq("__namespace__").Offset(1), get: []ds.PropertyMap{
					pmap("$key", mkKey("dev~app", "", "__namespace__", "bob")),
					pmap("$key", mkKey("dev~app", "", "__namespace__", "ns")),
				}},
				{q: nq("__namespace__").Offset(1).Limit(1), get: []ds.PropertyMap{
					pmap("$key", mkKey("dev~app", "", "__namespace__", "bob")),
				}},
				//
				// eventual consistency; Unique/1 is deleted at HEAD. Keysonly finds it,
				// but 'normal' doesn't.
				{q: nq("Unique").Gt("__key__", key("AKind", 5)).Lte("__key__", key("Zeta", "prime")),
					keys: []*ds.Key{key("Unique", 1)},
					get:  []ds.PropertyMap{}},

				// Filtering on nested entity properties.
				{q: nq("A").Eq("Inner.Prop", 2), get: []ds.PropertyMap{
					stage1Data[7],
				}},
				{q: nq("A").Eq("Inner.__key__", key("Inner", 2)), get: []ds.PropertyMap{
					stage1Data[6],
				}},
			},

			extraFns: []func(context.Context, *testing.T){
				func(c context.Context, t *testing.T) {
					q := nq("").Gt("__key__", key("Kind", 2))
					err := ds.Run(c, q, func(pm ds.PropertyMap) error {
						assert.Loosely(t, pm, should.Match(stage1Data[2]))
						return ds.Stop
					})
					assert.Loosely(t, err, shouldBeSuccessful)
				},

				func(c context.Context, t *testing.T) {
					q := nq("Something").Eq("Does", 2).Order("Not", "-Work")
					assert.Loosely(t, ds.Run(c, q, func(ds.Key) {}), should.ErrLike(strings.Join([]string{
						"Consider adding:",
						"- kind: Something",
						"  properties:",
						"  - name: Does",
						"  - name: Not",
						"  - name: Work",
						"    direction: desc",
					}, "\n")))
				},

				func(c context.Context, t *testing.T) {
					q := nq("Something").Ancestor(key("Kind", 3)).Order("Val")
					assert.Loosely(t, ds.Run(c, q, func(ds.Key) {}), should.ErrLike(strings.Join([]string{
						"Consider adding:",
						"- kind: Something",
						"  ancestor: true",
						"  properties:",
						"  - name: Val",
					}, "\n")))
				},
			},
		},

		{
			expect: []qExpect{
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

	{"regression: tombstones and limit/offset queries", []qExStage{
		{
			putEnts: []ds.PropertyMap{
				pmap("$key", key("Kind", 1)),
				pmap("$key", key("Kind", 2)),
				pmap("$key", key("Kind", 3)),
			},
			delEnts: []*ds.Key{key("Kind", 2)},
		},
		{
			expect: []qExpect{
				{
					q: nq("Kind").Limit(2),
					get: []ds.PropertyMap{
						pmap("$key", key("Kind", 1)),
						pmap("$key", key("Kind", 3)),
					},
				},

				{
					q:   nq("Kind").Offset(2),
					get: []ds.PropertyMap{},
				},
			},
		},
	}},

	{"regression: avoid index bleedover for common fields in compound indices", []qExStage{
		{
			addIdxs: []*ds.IndexDefinition{
				indx("Kind", "A", "B"),
				indx("Other", "A", "B"),
			},
			putEnts: []ds.PropertyMap{
				pmap(
					"$key", key("Kind", 1), Next,
					"A", "value", Next,
					"B", "value", Next),
			},
		},
		{
			expect: []qExpect{
				{
					q:   nq("Other").Eq("A", "value").Order("B"),
					get: []ds.PropertyMap{},
				},
			},
		},
	}},

	{"regression: don't expose __entity_group__ entities in kindless queries", []qExStage{
		{
			putEnts: []ds.PropertyMap{
				pmap(
					"$key", key("Kind", 1), Next,
					"A", "value"),
			},
		},
		{
			expect: []qExpect{
				{
					q: nq(""),
					// prior to the bug fix, this would have also shown the
					// __entity_group__ entity.
					get: []ds.PropertyMap{
						pmap(
							"$key", key("Kind", 1), Next,
							"A", "value"),
					},
				},
			},
		},
	}},
}

func TestQueryExecution(t *testing.T) {
	t.Parallel()

	ftt.Run("Test query execution", t, func(t *ftt.Test) {
		c, err := info.Namespace(Use(context.Background()), "ns")
		if err != nil {
			panic(err)
		}

		assert.Loosely(t, info.FullyQualifiedAppID(c), should.Equal("dev~app"))
		assert.Loosely(t, info.GetNamespace(c), should.Equal("ns"))

		testing := ds.GetTestable(c)

		for _, tc := range queryExecutionTests {
			t.Run(tc.name, func(t *ftt.Test) {
				for i, stage := range tc.test {
					// outside of Convey, since these must always happen
					testing.CatchupIndexes()

					testing.AddIndexes(stage.addIdxs...)
					byNs := map[string][]ds.PropertyMap{}
					for _, ent := range stage.putEnts {
						k := ds.GetMetaDefault(ent, "key", nil).(*ds.Key)
						byNs[k.Namespace()] = append(byNs[k.Namespace()], ent)
					}
					for ns, ents := range byNs {
						c := info.MustNamespace(c, ns)
						if err := ds.Put(c, ents); err != nil {
							// prevent Convey from thinking this assertion should show up in
							// every test loop.
							panic(err)
						}
					}

					if err := ds.Delete(c, stage.delEnts); err != nil {
						panic(err)
					}

					t.Run(fmt.Sprintf("stage %d", i), func(t *ftt.Test) {
						for j, expect := range stage.expect {
							runner := func(c context.Context, f func(ic context.Context) error, _ *ds.TransactionOptions) error {
								return f(c)
							}
							if expect.inTxn {
								runner = ds.RunInTransaction
							}

							if expect.count == 0 {
								if len(expect.keys) > 0 {
									expect.count = len(expect.keys)
								} else {
									expect.count = len(expect.get)
								}
							}

							if expect.keys != nil {
								t.Run(fmt.Sprintf("expect %d (keys)", j), func(t *ftt.Test) {
									err := runner(c, func(c context.Context) error {
										count, err := ds.Count(c, expect.q)
										assert.Loosely(t, err, shouldBeSuccessful)
										assert.Loosely(t, count, should.Equal(expect.count))

										rslt := []*ds.Key(nil)
										assert.Loosely(t, ds.GetAll(c, expect.q, &rslt), shouldBeSuccessful)
										assert.Loosely(t, len(rslt), should.Equal(len(expect.keys)))
										for i, r := range rslt {
											assert.Loosely(t, r, should.Match(expect.keys[i]))
										}
										return nil
									}, nil)
									assert.Loosely(t, err, shouldBeSuccessful)
								})
							}

							if expect.get != nil {
								t.Run(fmt.Sprintf("expect %d (data)", j), func(t *ftt.Test) {
									err := runner(c, func(c context.Context) error {
										count, err := ds.Count(c, expect.q)
										assert.Loosely(t, err, shouldBeSuccessful)
										assert.Loosely(t, count, should.Equal(expect.count))

										rslt := []ds.PropertyMap(nil)
										assert.Loosely(t, ds.GetAll(c, expect.q, &rslt), shouldBeSuccessful)
										assert.Loosely(t, len(rslt), should.Equal(len(expect.get)))
										for i, r := range rslt {
											assert.Loosely(t, r, should.Match(expect.get[i]))
										}
										return nil
									}, nil)
									assert.Loosely(t, err, shouldBeSuccessful)
								})
							}
						}

						for j, fn := range stage.extraFns {
							t.Run(fmt.Sprintf("extraFn %d", j), func(t *ftt.Test) {
								fn(c, t.T)
							})
						}
					})
				}
			})
		}
	})

	ftt.Run("Test AutoIndex", t, func(t *ftt.Test) {
		c, err := info.Namespace(Use(context.Background()), "ns")
		if err != nil {
			panic(err)
		}

		testing := ds.GetTestable(c)
		testing.Consistent(true)

		assert.Loosely(t, ds.Put(c, pmap("$key", key("Kind", 1), Next,
			"Val", 1, 2, 3, Next,
			"Extra", "hello",
		)), shouldBeSuccessful)

		assert.Loosely(t, ds.Put(c, pmap("$key", key("Kind", 2), Next,
			"Val", 2, 3, 9, Next,
			"Extra", "ace", "hello", "there",
		)), shouldBeSuccessful)

		q := nq("Kind").Gt("Val", 2).Order("Val", "Extra")

		count, err := ds.Count(c, q)
		assert.Loosely(t, err, should.ErrLike("Insufficient indexes"))

		testing.AutoIndex(true)

		count, err = ds.Count(c, q)
		assert.Loosely(t, err, shouldBeSuccessful)
		assert.Loosely(t, count, should.Equal(2))
	})
}

func shouldBeSuccessful(expected error) *failure.Summary {
	if expected == nil || expected == ds.Stop {
		return nil
	}
	return comparison.NewSummaryBuilder("shouldBeSuccessful").
		Actual(expected).
		Because("expected nil or ds.Stop").
		Summary
}
