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
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	ds "go.chromium.org/luci/gae/service/datastore"
	infoS "go.chromium.org/luci/gae/service/info"
)

type MetaGroup struct {
	_id    int64   `gae:"$id,1"`
	_kind  string  `gae:"$kind,__entity_group__"`
	Parent *ds.Key `gae:"$parent"`

	Version int64 `gae:"__version__"`
}

func testGetMeta(c context.Context, k *ds.Key) int64 {
	mg := &MetaGroup{Parent: k.Root()}
	if err := ds.Get(c, mg); err != nil {
		panic(err)
	}
	return mg.Version
}

type Nested struct {
	Inner int
}

type Foo struct {
	ID     int64   `gae:"$id"`
	Parent *ds.Key `gae:"$parent"`

	Val    int
	Name   string
	Multi  []string
	Key    *ds.Key
	Nested Nested `gae:",lsp"`

	Scatter []byte `gae:"__scatter__"` // this is normally invisible
}

func init() {
	registry.RegisterCmpOption(cmp.AllowUnexported(ds.Property{}))
}

func TestDatastoreSingleReadWriter(t *testing.T) {
	t.Parallel()

	ftt.Run("Datastore single reads and writes", t, func(t *ftt.Test) {
		c := Use(context.Background())
		assert.Loosely(t, ds.Raw(c), should.NotBeNil)

		t.Run("getting objects that DNE is an error", func(t *ftt.Test) {
			assert.Loosely(t, ds.Get(c, &Foo{ID: 1}), should.Equal(ds.ErrNoSuchEntity))
		})

		t.Run("bad namespaces fail", func(t *ftt.Test) {
			_, err := infoS.Namespace(c, "$$blzyall")
			assert.Loosely(t, err.Error(), should.ContainSubstring("namespace \"$$blzyall\" does not match"))
		})

		t.Run("Can Put stuff", func(t *ftt.Test) {
			// with an incomplete key!
			f := &Foo{
				Val:   10,
				Multi: []string{"foo", "bar"},
				Key:   ds.MakeKey(c, "Bar", "Baz"),
				Nested: Nested{
					Inner: 456,
				},
			}
			assert.Loosely(t, ds.Put(c, f), should.BeNil)
			k := ds.KeyForObj(c, f)
			assert.Loosely(t, k.String(), should.Equal("dev~app::/Foo,1"))

			t.Run("and Get it back", func(t *ftt.Test) {
				newFoo := &Foo{ID: 1}
				assert.Loosely(t, ds.Get(c, newFoo), should.BeNil)
				assert.Loosely(t, newFoo, should.Match(f))

				t.Run("but it's hidden from a different namespace", func(t *ftt.Test) {
					c, err := infoS.Namespace(c, "whombat")
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, ds.Get(c, f), should.Equal(ds.ErrNoSuchEntity))
				})

				t.Run("and we can Delete it", func(t *ftt.Test) {
					assert.Loosely(t, ds.Delete(c, k), should.BeNil)
					assert.Loosely(t, ds.Get(c, newFoo), should.Equal(ds.ErrNoSuchEntity))
				})

			})
			t.Run("Can Get it back as a PropertyMap", func(t *ftt.Test) {
				pmap := ds.PropertyMap{
					"$id":   propNI(1),
					"$kind": propNI("Foo"),
				}
				assert.Loosely(t, ds.Get(c, pmap), should.BeNil)
				assert.Loosely(t, pmap, should.Match(ds.PropertyMap{
					"$id":   propNI(1),
					"$kind": propNI("Foo"),
					"Name":  prop(""),
					"Val":   prop(10),
					"Multi": ds.PropertySlice{prop("foo"), prop("bar")},
					"Key":   prop(ds.MkKeyContext("dev~app", "").MakeKey("Bar", "Baz")),
					"Nested": prop(ds.PropertyMap{
						"Inner": prop(456),
					}),
				}))
			})
			t.Run("Deleting with a bogus key is bad", func(t *ftt.Test) {
				assert.Loosely(t, ds.Delete(c, ds.NewKey(c, "Foo", "wat", 100, nil)), should.ErrLike(ds.ErrInvalidKey))
			})
			t.Run("Deleting a DNE entity is fine", func(t *ftt.Test) {
				assert.Loosely(t, ds.Delete(c, ds.NewKey(c, "Foo", "wat", 0, nil)), should.BeNil)
			})

			t.Run("Deleting entities from a nonexistant namespace works", func(t *ftt.Test) {
				c := infoS.MustNamespace(c, "noexist")
				keys := make([]*ds.Key, 10)
				for i := range keys {
					keys[i] = ds.MakeKey(c, "Kind", i+1)
				}
				assert.Loosely(t, ds.Delete(c, keys), should.BeNil)
				count := 0
				assert.Loosely(t, ds.Raw(c).DeleteMulti(keys, func(idx int, err error) {
					assert.Loosely(t, idx, should.Equal(count))
					assert.Loosely(t, err, should.BeNil)

					count++
				}), should.BeNil)
				assert.Loosely(t, count, should.Equal(len(keys)))
			})

			t.Run("with multiple puts", func(t *ftt.Test) {
				assert.Loosely(t, testGetMeta(c, k), should.Equal(1))

				foos := make([]Foo, 10)
				for i := range foos {
					foos[i].Val = 10
					foos[i].Parent = k
				}
				assert.Loosely(t, ds.Put(c, foos), should.BeNil)
				assert.Loosely(t, testGetMeta(c, k), should.Equal(11))

				keys := make([]*ds.Key, len(foos))
				for i, f := range foos {
					keys[i] = ds.KeyForObj(c, &f)
				}

				t.Run("ensure that group versions persist across deletes", func(t *ftt.Test) {
					assert.Loosely(t, ds.Delete(c, append(keys, k)), should.BeNil)

					ds.GetTestable(c).CatchupIndexes()

					count := 0
					assert.Loosely(t, ds.Run(c, ds.NewQuery(""), func(_ *ds.Key) {
						count++
					}), should.BeNil)
					assert.Loosely(t, count, should.Equal(2))

					assert.Loosely(t, testGetMeta(c, k), should.Equal(22))

					assert.Loosely(t, ds.Put(c, &Foo{ID: 1}), should.BeNil)
					assert.Loosely(t, testGetMeta(c, k), should.Equal(23))
				})

				t.Run("can Get", func(t *ftt.Test) {
					vals := make([]ds.PropertyMap, len(keys))
					for i := range vals {
						vals[i] = ds.PropertyMap{}
						assert.Loosely(t, vals[i].SetMeta("key", keys[i]), should.BeTrue)
					}
					assert.Loosely(t, ds.Get(c, vals), should.BeNil)

					for i, val := range vals {
						assert.Loosely(t, val, should.Match(ds.PropertyMap{
							"Val":  ds.MkProperty(10),
							"Name": ds.MkProperty(""),
							"$key": ds.MkPropertyNI(keys[i]),
							"Key":  ds.MkProperty(nil),
							"Nested": ds.MkProperty(ds.PropertyMap{
								"Inner": ds.MkProperty(0),
							}),
						}))
					}
				})

			})

			t.Run("allocating ids prevents their use", func(t *ftt.Test) {
				keys := ds.NewIncompleteKeys(c, 100, "Foo", nil)
				assert.Loosely(t, ds.AllocateIDs(c, keys), should.BeNil)
				assert.Loosely(t, len(keys), should.Equal(100))

				// Assert that none of our keys share the same ID.
				ids := make(map[int64]struct{})
				for _, k := range keys {
					ids[k.IntID()] = struct{}{}
				}
				assert.Loosely(t, len(ids), should.Equal(len(keys)))

				// Put a new object and ensure that it is allocated an unused ID.
				f := &Foo{Val: 10}
				assert.Loosely(t, ds.Put(c, f), should.BeNil)
				k := ds.KeyForObj(c, f)
				assert.Loosely(t, k.String(), should.Equal("dev~app::/Foo,102"))

				_, ok := ids[k.IntID()]
				assert.Loosely(t, ok, should.BeFalse)
			})
		})

		t.Run("implements DSTransactioner", func(t *ftt.Test) {
			t.Run("Put", func(t *ftt.Test) {
				f := &Foo{Val: 10}
				assert.Loosely(t, ds.Put(c, f), should.BeNil)
				k := ds.KeyForObj(c, f)
				assert.Loosely(t, k.String(), should.Equal("dev~app::/Foo,1"))

				t.Run("can describe its transaction state", func(t *ftt.Test) {
					assert.Loosely(t, ds.CurrentTransaction(c), should.BeNil)

					err := ds.RunInTransaction(c, func(c context.Context) error {
						assert.Loosely(t, ds.CurrentTransaction(c), should.NotBeNil)

						// Can reset to nil.
						nc := ds.WithoutTransaction(c)
						assert.Loosely(t, ds.CurrentTransaction(nc), should.BeNil)
						return nil
					}, nil)
					assert.Loosely(t, err, should.BeNil)
				})

				t.Run("can Put new entity groups", func(t *ftt.Test) {
					err := ds.RunInTransaction(c, func(c context.Context) error {
						f := &Foo{Val: 100}
						assert.Loosely(t, ds.Put(c, f), should.BeNil)
						assert.Loosely(t, f.ID, should.Equal(2))

						f.ID = 0
						f.Val = 200
						assert.Loosely(t, ds.Put(c, f), should.BeNil)
						assert.Loosely(t, f.ID, should.Equal(3))

						return nil
					}, nil)
					assert.Loosely(t, err, should.BeNil)

					f := &Foo{ID: 2}
					assert.Loosely(t, ds.Get(c, f), should.BeNil)
					assert.Loosely(t, f.Val, should.Equal(100))

					f.ID = 3
					assert.Loosely(t, ds.Get(c, f), should.BeNil)
					assert.Loosely(t, f.Val, should.Equal(200))
				})

				t.Run("can Put new entities in a current group", func(t *ftt.Test) {
					err := ds.RunInTransaction(c, func(c context.Context) error {
						f := &Foo{Val: 100, Parent: k}
						assert.Loosely(t, ds.Put(c, f), should.BeNil)
						assert.Loosely(t, ds.KeyForObj(c, f).String(), should.Equal("dev~app::/Foo,1/Foo,1"))

						f.ID = 0
						f.Val = 200
						assert.Loosely(t, ds.Put(c, f), should.BeNil)
						assert.Loosely(t, ds.KeyForObj(c, f).String(), should.Equal("dev~app::/Foo,1/Foo,2"))

						return nil
					}, nil)
					assert.Loosely(t, err, should.BeNil)

					f := &Foo{ID: 1, Parent: k}
					assert.Loosely(t, ds.Get(c, f), should.BeNil)
					assert.Loosely(t, f.Val, should.Equal(100))

					f.ID = 2
					assert.Loosely(t, ds.Get(c, f), should.BeNil)
					assert.Loosely(t, f.Val, should.Equal(200))
				})

				t.Run("Deletes work too", func(t *ftt.Test) {
					err := ds.RunInTransaction(c, func(c context.Context) error {
						return ds.Delete(c, k)
					}, nil)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, ds.Get(c, &Foo{ID: 1}), should.Equal(ds.ErrNoSuchEntity))
				})

				t.Run("Get takes a snapshot", func(t *ftt.Test) {
					err := ds.RunInTransaction(c, func(c context.Context) error {
						assert.Loosely(t, ds.Get(c, f), should.BeNil)
						assert.Loosely(t, f.Val, should.Equal(10))

						// Don't ever do this in a real program unless you want to guarantee
						// a failed transaction :)
						f.Val = 11
						assert.Loosely(t, ds.Put(ds.WithoutTransaction(c), f), should.BeNil)

						assert.Loosely(t, ds.Get(c, f), should.BeNil)
						assert.Loosely(t, f.Val, should.Equal(10))

						return nil
					}, nil)
					assert.Loosely(t, err, should.BeNil)

					f := &Foo{ID: 1}
					assert.Loosely(t, ds.Get(c, f), should.BeNil)
					assert.Loosely(t, f.Val, should.Equal(11))
				})

				t.Run("and snapshots are consistent even after Puts", func(t *ftt.Test) {
					err := ds.RunInTransaction(c, func(c context.Context) error {
						f := &Foo{ID: 1}
						assert.Loosely(t, ds.Get(c, f), should.BeNil)
						assert.Loosely(t, f.Val, should.Equal(10))

						// Don't ever do this in a real program unless you want to guarantee
						// a failed transaction :)
						f.Val = 11
						assert.Loosely(t, ds.Put(ds.WithoutTransaction(c), f), should.BeNil)

						assert.Loosely(t, ds.Get(c, f), should.BeNil)
						assert.Loosely(t, f.Val, should.Equal(10))

						f.Val = 20
						assert.Loosely(t, ds.Put(c, f), should.BeNil)

						assert.Loosely(t, ds.Get(c, f), should.BeNil)
						assert.Loosely(t, f.Val, should.Equal(10)) // still gets 10

						return nil
					}, &ds.TransactionOptions{Attempts: 1})
					assert.Loosely(t, err.Error(), should.ContainSubstring("concurrent"))

					f := &Foo{ID: 1}
					assert.Loosely(t, ds.Get(c, f), should.BeNil)
					assert.Loosely(t, f.Val, should.Equal(11))
				})

				t.Run("Reusing a transaction context is bad news", func(t *ftt.Test) {
					var txnCtx context.Context
					err := ds.RunInTransaction(c, func(c context.Context) error {
						txnCtx = c
						assert.Loosely(t, ds.Get(c, f), should.BeNil)
						return nil
					}, nil)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, ds.Get(txnCtx, f).Error(), should.ContainSubstring("expired"))
				})

				t.Run("Nested transactions are rejected", func(t *ftt.Test) {
					err := ds.RunInTransaction(c, func(c context.Context) error {
						err := ds.RunInTransaction(c, func(c context.Context) error {
							panic("noooo")
						}, nil)
						assert.Loosely(t, err.Error(), should.ContainSubstring("nested transactions"))
						return nil
					}, nil)
					assert.Loosely(t, err, should.BeNil)
				})

				t.Run("Transactions can be escaped.", func(t *ftt.Test) {
					testError := errors.New("test error")
					noTxnPM := ds.PropertyMap{
						"$kind": ds.MkProperty("Test"),
						"$id":   ds.MkProperty("no txn"),
					}

					err := ds.RunInTransaction(c, func(c context.Context) error {
						assert.Loosely(t, ds.CurrentTransaction(c), should.NotBeNil)

						pmap := ds.PropertyMap{
							"$kind": ds.MkProperty("Test"),
							"$id":   ds.MkProperty("quux"),
						}
						if err := ds.Put(c, pmap); err != nil {
							return err
						}

						// Put an entity outside of the transaction so we can confirm that
						// it was added even when the transaction fails.
						if err := ds.Put(ds.WithoutTransaction(c), noTxnPM); err != nil {
							return err
						}
						return testError
					}, nil)
					assert.Loosely(t, err, should.Equal(testError))

					// Confirm that noTxnPM was added.
					assert.Loosely(t, ds.CurrentTransaction(c), should.BeNil)
					assert.Loosely(t, ds.Get(c, noTxnPM), should.BeNil)
				})

				t.Run("Concurrent transactions only accept one set of changes", func(t *ftt.Test) {
					// Note: I think this implementation is actually /slightly/ wrong.
					// According to my read of the docs for appengine, when you open a
					// transaction it actually (essentially) holds a reference to the
					// entire datastore. Our implementation takes a snapshot of the
					// entity group as soon as something observes/affects it.
					//
					// That said... I'm not sure if there's really a semantic difference.
					err := ds.RunInTransaction(c, func(c context.Context) error {
						assert.Loosely(t, ds.Put(c, &Foo{ID: 1, Val: 21}), should.BeNil)

						err := ds.RunInTransaction(ds.WithoutTransaction(c), func(c context.Context) error {
							assert.Loosely(t, ds.Put(c, &Foo{ID: 1, Val: 27}), should.BeNil)
							return nil
						}, nil)
						assert.Loosely(t, err, should.BeNil)

						return nil
					}, nil)
					assert.Loosely(t, err.Error(), should.ContainSubstring("concurrent"))

					f := &Foo{ID: 1}
					assert.Loosely(t, ds.Get(c, f), should.BeNil)
					assert.Loosely(t, f.Val, should.Equal(27))
				})

				t.Run("Errors and panics", func(t *ftt.Test) {
					t.Run("returning an error aborts", func(t *ftt.Test) {
						err := ds.RunInTransaction(c, func(c context.Context) error {
							assert.Loosely(t, ds.Put(c, &Foo{ID: 1, Val: 200}), should.BeNil)
							return fmt.Errorf("thingy")
						}, nil)
						assert.Loosely(t, err.Error(), should.Equal("thingy"))

						f := &Foo{ID: 1}
						assert.Loosely(t, ds.Get(c, f), should.BeNil)
						assert.Loosely(t, f.Val, should.Equal(10))
					})

					t.Run("panicing aborts", func(t *ftt.Test) {
						assert.Loosely(t, func() {
							assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
								assert.Loosely(t, ds.Put(c, &Foo{Val: 200}), should.BeNil)
								panic("wheeeeee")
							}, nil), should.BeNil)
						}, should.Panic)

						f := &Foo{ID: 1}
						assert.Loosely(t, ds.Get(c, f), should.BeNil)
						assert.Loosely(t, f.Val, should.Equal(10))
					})
				})

				t.Run("Transaction retries", func(t *ftt.Test) {
					tst := ds.GetTestable(c)
					defer func() { tst.SetTransactionRetryCount(0) }()

					t.Run("SetTransactionRetryCount set to zero", func(t *ftt.Test) {
						tst.SetTransactionRetryCount(0)
						calls := 0
						assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
							calls++
							return nil
						}, nil), should.BeNil)
						assert.Loosely(t, calls, should.Equal(1))
					})

					t.Run("default TransactionOptions is 3 attempts", func(t *ftt.Test) {
						tst.SetTransactionRetryCount(100) // more than 3
						calls := 0
						assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
							calls++
							return nil
						}, nil), should.Equal(ds.ErrConcurrentTransaction))
						assert.Loosely(t, calls, should.Equal(3))
					})

					t.Run("non-default TransactionOptions ", func(t *ftt.Test) {
						tst.SetTransactionRetryCount(100) // more than 20
						calls := 0
						assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
							calls++
							return nil
						}, &ds.TransactionOptions{Attempts: 20}), should.Equal(ds.ErrConcurrentTransaction))
						assert.Loosely(t, calls, should.Equal(20))
					})

					t.Run("SetTransactionRetryCount is respected", func(t *ftt.Test) {
						tst.SetTransactionRetryCount(1) // less than 3
						calls := 0
						assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
							calls++
							return nil
						}, nil), should.BeNil)
						assert.Loosely(t, calls, should.Equal(2))
					})

					t.Run("fatal errors are not retried", func(t *ftt.Test) {
						tst.SetTransactionRetryCount(1)
						calls := 0
						assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
							calls++
							return fmt.Errorf("omg")
						}, nil).Error(), should.Equal("omg"))
						assert.Loosely(t, calls, should.Equal(1))
					})
				})

				t.Run("Read-only transactions reject writes", func(t *ftt.Test) {
					assert.Loosely(t, ds.Put(c, &Foo{ID: 1, Val: 100}), should.BeNil)
					var val int

					assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
						foo := &Foo{ID: 1}
						assert.Loosely(t, ds.Get(c, foo), should.BeNil)
						val = foo.Val

						foo.Val = 1337
						return ds.Put(c, foo)
					}, &ds.TransactionOptions{ReadOnly: true}), should.ErrLike("Attempting to write"))
					assert.Loosely(t, val, should.Match(100))
					foo := &Foo{ID: 1}
					assert.Loosely(t, ds.Get(c, foo), should.BeNil)
					assert.Loosely(t, foo.Val, should.Match(100))
				})

				t.Run("Read-only transactions reject deletes", func(t *ftt.Test) {
					assert.Loosely(t, ds.Put(c, &Foo{ID: 1, Val: 100}), should.BeNil)
					var val int

					assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
						foo := &Foo{ID: 1}
						assert.Loosely(t, ds.Get(c, foo), should.BeNil)
						val = foo.Val

						return ds.Delete(c, foo)
					}, &ds.TransactionOptions{ReadOnly: true}), should.ErrLike("Attempting to delete"))
					assert.Loosely(t, val, should.Match(100))
					assert.Loosely(t, ds.Get(c, &Foo{ID: 1}), should.BeNil)
				})

				t.Run("Allocates IDs eagerly by default", func(t *ftt.Test) {
					err := ds.RunInTransaction(c, func(c context.Context) error {
						f := &Foo{Val: 100}
						assert.That(t, ds.KeyForObj(c, f).IsIncomplete(), should.BeTrue)
						assert.Loosely(t, ds.Put(c, f), should.BeNil)
						assert.That(t, ds.KeyForObj(c, f).IsIncomplete(), should.BeFalse)
						assert.Loosely(t, f.ID, should.Equal(2))
						return nil
					}, nil)
					assert.Loosely(t, err, should.BeNil)

					f := &Foo{ID: 2}
					assert.Loosely(t, ds.Get(c, f), should.BeNil)
					assert.Loosely(t, f.Val, should.Equal(100))
				})

				t.Run("Doesn't allocate IDs when AllocateIDsOnCommit is true", func(t *ftt.Test) {
					err := ds.RunInTransaction(c, func(c context.Context) error {
						f := &Foo{Val: 100}
						assert.That(t, ds.KeyForObj(c, f).IsIncomplete(), should.BeTrue)
						assert.Loosely(t, ds.Put(c, f), should.BeNil)
						assert.That(t, ds.KeyForObj(c, f).IsIncomplete(), should.BeTrue)
						return nil
					}, &ds.TransactionOptions{
						AllocateIDsOnCommit: true,
					})
					assert.Loosely(t, err, should.BeNil)

					// Still actually stored it. But this ID is only "guessable" right
					// now. There's no way to get it back.
					f := &Foo{ID: 2}
					assert.Loosely(t, ds.Get(c, f), should.BeNil)
					assert.Loosely(t, f.Val, should.Equal(100))
				})
			})
		})

		t.Run("Testable.Consistent", func(t *ftt.Test) {
			t.Run("false", func(t *ftt.Test) {
				ds.GetTestable(c).Consistent(false) // the default
				for i := 0; i < 10; i++ {
					assert.Loosely(t, ds.Put(c, &Foo{ID: int64(i + 1), Val: i + 1}), should.BeNil)
				}
				q := ds.NewQuery("Foo").Gt("Val", 3)
				count, err := ds.Count(c, q)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, count, should.BeZero)

				assert.Loosely(t, ds.Delete(c, ds.MakeKey(c, "Foo", 4)), should.BeNil)

				count, err = ds.Count(c, q)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, count, should.BeZero)

				ds.GetTestable(c).Consistent(true)
				count, err = ds.Count(c, q)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, count, should.Equal(6))
			})

			t.Run("true", func(t *ftt.Test) {
				ds.GetTestable(c).Consistent(true)
				for i := 0; i < 10; i++ {
					assert.Loosely(t, ds.Put(c, &Foo{ID: int64(i + 1), Val: i + 1}), should.BeNil)
				}
				q := ds.NewQuery("Foo").Gt("Val", 3)
				count, err := ds.Count(c, q)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, count, should.Equal(7))

				assert.Loosely(t, ds.Delete(c, ds.MakeKey(c, "Foo", 4)), should.BeNil)

				count, err = ds.Count(c, q)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, count, should.Equal(6))
			})
		})

		t.Run("Testable.DisableSpecialEntities", func(t *ftt.Test) {
			ds.GetTestable(c).DisableSpecialEntities(true)

			assert.Loosely(t, ds.Put(c, &Foo{}), should.ErrLike("allocateIDs is disabled"))

			assert.Loosely(t, ds.Put(c, &Foo{ID: 1}), should.BeNil)

			ds.GetTestable(c).CatchupIndexes()

			count, err := ds.Count(c, ds.NewQuery(""))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, count, should.Equal(1)) // normally this would include __entity_group__
		})

		t.Run("Datastore namespace interaction", func(t *ftt.Test) {
			run := func(rc context.Context, txn bool) (putErr, getErr, queryErr, countErr error) {
				var foo Foo

				putFunc := func(doC context.Context) error {
					return ds.Put(doC, &foo)
				}

				doFunc := func(doC context.Context) {
					getErr = ds.Get(doC, &foo)

					q := ds.NewQuery("Foo").Ancestor(ds.KeyForObj(doC, &foo))
					queryErr = ds.Run(doC, q, func(f *Foo) error { return nil })
					_, countErr = ds.Count(doC, q)
				}

				if txn {
					putErr = ds.RunInTransaction(rc, func(ic context.Context) error {
						return putFunc(ic)
					}, nil)
					if putErr != nil {
						return
					}

					ds.GetTestable(rc).CatchupIndexes()
					ds.RunInTransaction(rc, func(ic context.Context) error {
						doFunc(ic)
						return nil
					}, nil)
				} else {
					putErr = putFunc(rc)
					if putErr != nil {
						return
					}
					ds.GetTestable(rc).CatchupIndexes()
					doFunc(rc)
				}
				return
			}

			for _, txn := range []bool{false, true} {
				t.Run(fmt.Sprintf("In transaction? %v", txn), func(t *ftt.Test) {
					t.Run("With no namespace installed, can Put, Get, Query, and Count.", func(t *ftt.Test) {
						assert.Loosely(t, infoS.GetNamespace(c), should.BeEmpty)

						putErr, getErr, queryErr, countErr := run(c, txn)
						assert.Loosely(t, putErr, should.BeNil)
						assert.Loosely(t, getErr, should.BeNil)
						assert.Loosely(t, queryErr, should.BeNil)
						assert.Loosely(t, countErr, should.BeNil)
					})

					t.Run("With a namespace installed, can Put, Get, Query, and Count.", func(t *ftt.Test) {
						putErr, getErr, queryErr, countErr := run(infoS.MustNamespace(c, "foo"), txn)
						assert.Loosely(t, putErr, should.BeNil)
						assert.Loosely(t, getErr, should.BeNil)
						assert.Loosely(t, queryErr, should.BeNil)
						assert.Loosely(t, countErr, should.BeNil)
					})
				})
			}
		})

		t.Run("Testable.ShowSpecialProperties", func(t *ftt.Test) {
			ds.GetTestable(c).ShowSpecialProperties(true)

			var ents []Foo
			for i := 0; i < 10; i++ {
				ent := &Foo{}
				assert.Loosely(t, ds.Put(c, ent), should.BeNil)
				ents = append(ents, *ent)
			}
			assert.Loosely(t, ds.Get(c, ents), should.BeNil)

			// Some of these entities (~50%) should have __scatter__ property
			// populated. The algorithm is deterministic.
			scatter := make([]string, len(ents))
			for i, e := range ents {
				scatter[i] = hex.EncodeToString(e.Scatter)
			}
			assert.Loosely(t, scatter, should.Match([]string{
				"d77e219d0669b1808f236ca5b25127bf8e865e3f0e68b792374526251c873c61",
				"",
				"",
				"",
				"",
				"",
				"b592c9de652ffc3f458910247fc16690ba2ceeef20a8566fda5dd989a5fc160e",
				"bcefad8a2212ee1cfa3636e94264b8c73c90eaded9f429e27c7384830c1e381c",
				"d2358c1d9e5951be7117e06eaec96a6a63090f181615e2c51afaf7f214e4d873",
				"b29a46a6c01adb88d7001fe399d6346d5d2725b190f4fb025c9cb7c73c4ffb15",
			}))
		})

		t.Run("Query by __scatter__", func(t *ftt.Test) {
			for i := 0; i < 100; i++ {
				assert.Loosely(t, ds.Put(c, &Foo{}), should.BeNil)
			}
			ds.GetTestable(c).CatchupIndexes()

			var ids []int64
			assert.Loosely(t, ds.Run(c, ds.NewQuery("Foo").Order("__scatter__").Limit(5), func(f *Foo) {
				assert.Loosely(t, f.Scatter, should.BeNil) // it is "invisible"
				ids = append(ids, f.ID)
			}), should.BeNil)

			// Approximately "even" distribution within [1, 100] range.
			assert.Loosely(t, ids, should.Match([]int64{43, 55, 99, 23, 17}))
		})
	})
}

func TestCompoundIndexes(t *testing.T) {
	t.Parallel()

	idxKey := func(def ds.IndexDefinition) string {
		assert.Loosely(t, def, should.NotBeZero)
		return "idx::" + string(ds.Serialize.ToBytes(*def.PrepForIdxTable()))
	}

	ftt.Run("Test Compound indexes", t, func(t *ftt.Test) {
		type Model struct {
			ID int64 `gae:"$id"`

			Field1 []string
			Field2 []int64
		}

		c := Use(context.Background())
		testImpl := ds.GetTestable(c).(*dsImpl)
		head := testImpl.data.head

		assert.Loosely(t, ds.Put(c, &Model{1, []string{"hello", "world"}, []int64{10, 11}}), should.BeNil)

		idx := ds.IndexDefinition{
			Kind: "Model",
			SortBy: []ds.IndexColumn{
				{Property: "Field2"},
			},
		}

		coll := head.Snapshot().GetCollection(idxKey(idx))
		assert.Loosely(t, coll, should.NotBeNil)
		assert.Loosely(t, countItems(coll), should.Equal(2))

		idx.SortBy[0].Property = "Field1"
		coll = head.Snapshot().GetCollection(idxKey(idx))
		assert.Loosely(t, coll, should.NotBeNil)
		assert.Loosely(t, countItems(coll), should.Equal(2))

		idx.SortBy = append(idx.SortBy, ds.IndexColumn{Property: "Field1"})
		assert.Loosely(t, head.GetCollection(idxKey(idx)), should.BeNil)

		testImpl.AddIndexes(&idx)
		coll = head.Snapshot().GetCollection(idxKey(idx))
		assert.Loosely(t, coll, should.NotBeNil)
		assert.Loosely(t, countItems(coll), should.Equal(4))
	})
}

// High level test for regression in how zero time is stored,
// see https://codereview.chromium.org/1334043003/
func TestDefaultTimeField(t *testing.T) {
	t.Parallel()

	ftt.Run("Default time.Time{} can be stored", t, func(t *ftt.Test) {
		type Model struct {
			ID   int64 `gae:"$id"`
			Time time.Time
		}
		c := Use(context.Background())
		m := Model{ID: 1}
		assert.Loosely(t, ds.Put(c, &m), should.BeNil)

		// Reset to something non zero to ensure zero is fetched.
		m.Time = time.Now().UTC()
		assert.Loosely(t, ds.Get(c, &m), should.BeNil)
		assert.Loosely(t, m.Time.IsZero(), should.BeTrue)
	})
}

func TestNewDatastore(t *testing.T) {
	t.Parallel()

	ftt.Run("Can get and use a NewDatastore", t, func(t *ftt.Test) {
		c := UseWithAppID(context.Background(), "dev~aid")
		c = infoS.MustNamespace(c, "ns")

		dsInst := NewDatastore(c, infoS.Raw(c))
		c = ds.SetRaw(c, dsInst)

		k := ds.MakeKey(c, "Something", 1)
		assert.Loosely(t, k.AppID(), should.Equal("dev~aid"))
		assert.Loosely(t, k.Namespace(), should.Equal("ns"))

		type Model struct {
			ID    int64 `gae:"$id"`
			Value []int64
		}
		assert.Loosely(t, ds.Put(c, &Model{ID: 1, Value: []int64{20, 30}}), should.BeNil)

		vals := []ds.PropertyMap{}
		assert.Loosely(t, ds.GetAll(c, ds.NewQuery("Model").Project("Value"), &vals), should.BeNil)
		assert.Loosely(t, len(vals), should.Equal(2))

		assert.Loosely(t, vals[0].Slice("Value")[0].Value(), should.Equal(20))
		assert.Loosely(t, vals[1].Slice("Value")[0].Value(), should.Equal(30))
	})
}

func TestAddIndexes(t *testing.T) {
	t.Parallel()

	ftt.Run("Test Testable.AddIndexes", t, func(t *ftt.Test) {
		ctx := UseWithAppID(context.Background(), "aid")
		namespaces := []string{"", "good", "news", "everyone"}

		t.Run("After adding datastore entries, can query against indexes in various namespaces", func(t *ftt.Test) {
			foos := []*Foo{
				{ID: 1, Val: 1, Name: "foo"},
				{ID: 2, Val: 2, Name: "bar"},
				{ID: 3, Val: 2, Name: "baz"},
			}
			for _, ns := range namespaces {
				assert.Loosely(t, ds.Put(infoS.MustNamespace(ctx, ns), foos), should.BeNil)
			}

			// Initial query, no indexes, will fail.
			ds.GetTestable(ctx).CatchupIndexes()

			var results []*Foo
			q := ds.NewQuery("Foo").Eq("Val", 2).Gte("Name", "bar")
			assert.Loosely(t, ds.GetAll(ctx, q, &results), should.ErrLike("Insufficient indexes"))

			// Add index for default namespace.
			ds.GetTestable(ctx).AddIndexes(&ds.IndexDefinition{
				Kind: "Foo",
				SortBy: []ds.IndexColumn{
					{Property: "Val"},
					{Property: "Name"},
				},
			})
			ds.GetTestable(ctx).CatchupIndexes()

			for _, ns := range namespaces {
				if ns == "" {
					// Skip query test for empty namespace, as this is invalid.
					continue
				}

				results = nil
				assert.Loosely(t, ds.GetAll(infoS.MustNamespace(ctx, ns), q, &results), should.BeNil)
				assert.Loosely(t, len(results), should.Equal(2))
			}

			// Add "foos" to a new namespace, then confirm that it gets indexed.
			assert.Loosely(t, ds.Put(infoS.MustNamespace(ctx, "qux"), foos), should.BeNil)
			ds.GetTestable(ctx).CatchupIndexes()

			results = nil
			assert.Loosely(t, ds.GetAll(infoS.MustNamespace(ctx, "qux"), q, &results), should.BeNil)
			assert.Loosely(t, len(results), should.Equal(2))
		})
	})
}

func TestConcurrentTxn(t *testing.T) {
	t.Parallel()

	// Stress test for concurrent transactions. It transactionally increments a
	// counter in an entity and counts how many transactions succeeded. The final
	// counter value and the number of committed transactions should match.

	ftt.Run("Concurrent transactions work", t, func(t *ftt.Test) {
		c := Use(context.Background())

		var successes int64

		for round := 0; round < 1000; round++ {
			barrier := make(chan struct{})
			wg := sync.WaitGroup{}

			for track := 0; track < 5; track++ {
				wg.Add(1)
				go func(round, track int) {
					defer wg.Done()
					<-barrier

					err := ds.RunInTransaction(c, func(c context.Context) error {
						ent := Foo{ID: 1}
						switch err := ds.Get(c, &ent); {
						case err == ds.ErrNoSuchEntity:
							// new entity
						case err != nil:
							return err
						}
						ent.Val++
						return ds.Put(c, &ent)
					}, nil)
					if err == nil {
						atomic.AddInt64(&successes, 1)
					}

				}(round, track)
			}

			// Run one round of the test.
			close(barrier)
			wg.Wait()

			// Verify that everything is still ok.
			ent := Foo{ID: 1}
			ds.Get(c, &ent)
			counter := atomic.LoadInt64(&successes)
			if int64(ent.Val) != counter { // don't spam convey assertions
				assert.Loosely(t, ent.Val, should.Equal(counter))
			}
		}
	})
}
