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

// adapted from github.com/golang/appengine/datastore

package datastore

import (
	"bytes"
	"container/heap"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/gae/service/info"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

var (
	errFail    = errors.New("Individual element fail")
	errFailAll = errors.New("Operation fail")
)

type fakeDatastore struct {
	RawInterface

	kctx         KeyContext
	keyForResult func(int32, KeyContext) *Key
	onDelete     func(*Key)
	entities     int32
	constraints  Constraints
	t            testing.TB
}

func (f *fakeDatastore) factory() RawFactory {
	return func(ic context.Context) RawInterface {
		fds := *f
		fds.kctx = GetKeyContext(ic)
		return &fds
	}
}

func (f *fakeDatastore) AllocateIDs(keys []*Key, cb NewKeyCB) error {
	if keys[0].Kind() == "FailAll" {
		return errFailAll
	}
	for i, k := range keys {
		if k.Kind() == "Fail" {
			cb(i, nil, errFail)
		} else {
			cb(i, f.kctx.NewKey(k.Kind(), "", int64(i+1), k.Parent()), nil)
		}
	}
	return nil
}

func (f *fakeDatastore) Run(fq *FinalizedQuery, cb RawRunCB) error {
	cur := int32(0)

	start, end := fq.Bounds()
	if start != nil {
		cur = int32(start.(fakeCursor))
	}

	remaining := int32(f.entities - cur)
	if end != nil {
		endV := int32(end.(fakeCursor))
		if remaining > endV {
			remaining = endV
		}
	}
	lim, _ := fq.Limit()
	if lim <= 0 || lim > remaining {
		lim = remaining
	}

	cursCB := func() (Cursor, error) {
		return fakeCursor(cur), nil
	}

	kfr := f.keyForResult
	if kfr == nil {
		kfr = func(i int32, kctx KeyContext) *Key { return kctx.MakeKey("Kind", (i + 1)) }
	}

	for i := int32(0); i < lim; i++ {
		if v, ok := fq.eqFilts["@err_single"]; ok {
			idx := fq.eqFilts["@err_single_idx"][0].Value().(int64)
			if idx == int64(i) {
				return errors.New(v[0].Value().(string))
			}
		}

		k := kfr(cur, f.kctx)
		pm := PropertyMap{"Value": MkProperty(cur)}
		cur++

		if err := cb(k, pm, cursCB); err != nil {
			if err == Stop {
				err = nil
			}
			return err
		}
	}
	return nil
}

func (f *fakeDatastore) PutMulti(keys []*Key, vals []PropertyMap, cb NewKeyCB) error {
	if keys[0].Kind() == "FailAll" {
		return errFailAll
	}
	_, assertExtra := vals[0].GetMeta("assertExtra")
	for i, k := range keys {
		err := error(nil)
		if k.Kind() == "Fail" {
			err = errFail
		} else {
			if assertExtra {
				assert.That(f.t, vals[i].Slice("Extra"), should.Resemble(PropertySlice{MkProperty("whoa")}))
			}

			if k.Kind() == "Index" {
				// Index types have a Value field. Generate a Key whose IntID equals
				// that Value field.
				k = k.KeyContext().NewKey(k.Kind(), "", vals[i]["Value"].Slice()[0].Value().(int64), k.Parent())
			} else {
				assert.That(f.t, vals[i].Slice("Value"), should.Resemble(PropertySlice{MkProperty(i)}))
				if k.IsIncomplete() {
					k = k.KeyContext().NewKey(k.Kind(), "", int64(i+1), k.Parent())
				}
			}
		}
		cb(i, k, err)
	}
	return nil
}

const noSuchEntityID = 0xdead

func (f *fakeDatastore) GetMulti(keys []*Key, _meta MultiMetaGetter, cb GetMultiCB) error {
	if keys[0].Kind() == "FailAll" {
		return errFailAll
	}
	for i, k := range keys {
		if k.Kind() == "Fail" {
			cb(i, nil, errFail)
		} else if k.Kind() == "DNE" || k.IntID() == noSuchEntityID {
			cb(i, nil, ErrNoSuchEntity)
		} else if k.Kind() == "Index" {
			cb(i, PropertyMap{"Value": MkProperty(k.IntID())}, nil)
		} else {
			cb(i, PropertyMap{"Value": MkProperty(i + 1)}, nil)
		}
	}
	return nil
}

func (f *fakeDatastore) DeleteMulti(keys []*Key, cb DeleteMultiCB) error {
	if keys[0].Kind() == "FailAll" {
		return errFailAll
	}
	for i, k := range keys {
		if k.Kind() == "Fail" {
			cb(i, errFail)
		} else if k.Kind() == "DNE" || k.IntID() == noSuchEntityID {
			cb(i, ErrNoSuchEntity)
		} else {
			cb(i, nil)
			if f.onDelete != nil {
				f.onDelete(k)
			}
		}
	}
	return nil
}

func (f *fakeDatastore) RunInTransaction(fu func(c context.Context) error, opts *TransactionOptions) error {
	return nil
}

func (f *fakeDatastore) Constraints() Constraints {
	return f.constraints
}

type badStruct struct {
	ID    int64     `gae:"$id"`
	Compy complex64 // bad type
}

type CommonStruct struct {
	ID     int64 `gae:"$id"`
	Parent *Key  `gae:"$parent"`

	Value int64
}

type ConstIDStruct struct {
	_id    int64 `gae:"$id,1"`
	Parent *Key  `gae:"$parent"`
	Value  int64
}

type PrivateStruct struct {
	Value int64

	_id int64 `gae:"$id"`
}

type permaBad struct {
	PropertyLoadSaver
	MetaGetterSetter
}

func (f *permaBad) Load(pm PropertyMap) error {
	return errors.New("permaBad")
}

type SingletonStruct struct {
	id int64 `gae:"$id,1"`
}

type FakePLS struct {
	IntID    int64
	StringID string
	Kind     string

	Value     int64
	gotLoaded bool

	t testing.TB

	failGetMeta bool
	failLoad    bool
	failProblem bool
	failSave    bool
	failSetMeta bool
}

var _ PropertyLoadSaver = (*FakePLS)(nil)

func (f *FakePLS) Load(pm PropertyMap) error {
	if f.failLoad {
		return errors.New("FakePLS.Load")
	}
	f.gotLoaded = true
	f.Value = pm.Slice("Value")[0].Value().(int64)
	return nil
}

func (f *FakePLS) Save(withMeta bool) (PropertyMap, error) {
	if f.failSave {
		return nil, errors.New("FakePLS.Save")
	}
	ret := PropertyMap{
		"Value": MkProperty(f.Value),
		"Extra": MkProperty("whoa"),
	}
	if withMeta {
		id, _ := f.GetMeta("id")
		assert.Loosely(f.t, ret.SetMeta("id", id), should.BeTrue)
		if f.Kind == "" {
			assert.Loosely(f.t, ret.SetMeta("kind", "FakePLS"), should.BeTrue)
		} else {
			assert.Loosely(f.t, ret.SetMeta("kind", f.Kind), should.BeTrue)
		}
		assert.Loosely(f.t, ret.SetMeta("assertExtra", true), should.BeTrue)
	}
	return ret, nil
}

func (f *FakePLS) GetMeta(key string) (any, bool) {
	if f.failGetMeta {
		return nil, false
	}
	switch key {
	case "id":
		if f.StringID != "" {
			return f.StringID, true
		}
		return f.IntID, true
	case "kind":
		if f.Kind == "" {
			return "FakePLS", true
		}
		return f.Kind, true
	}
	return nil, false
}

func (f *FakePLS) GetAllMeta() PropertyMap {
	ret := PropertyMap{}
	if id, ok := f.GetMeta("id"); !ok {
		ret.SetMeta("id", id)
	}
	if kind, ok := f.GetMeta("kind"); !ok {
		ret.SetMeta("kind", kind)
	}
	return ret
}

func (f *FakePLS) SetMeta(key string, val any) bool {
	if f.failSetMeta {
		return false
	}
	if key == "id" {
		switch x := val.(type) {
		case int64:
			f.IntID = x
		case string:
			f.StringID = x
		}
		return true
	}
	if key == "kind" {
		f.Kind = val.(string)
		return true
	}
	return false
}

func (f *FakePLS) Problem() error {
	if f.failProblem {
		return errors.New("FakePLS.Problem")
	}
	return nil
}

// plsChan to test channel PLS types.
type plsChan chan Property

var _ PropertyLoadSaver = plsChan(nil)

func (c plsChan) Load(pm PropertyMap) error               { return nil }
func (c plsChan) Save(withMeta bool) (PropertyMap, error) { return nil, nil }
func (c plsChan) SetMeta(key string, val any) bool        { return false }
func (c plsChan) Problem() error                          { return nil }

func (c plsChan) GetMeta(key string) (any, bool) {
	switch key {
	case "kind":
		return "plsChan", true
	case "id":
		return "whyDoIExist", true
	}
	return nil, false
}

func (c plsChan) GetAllMeta() PropertyMap {
	return PropertyMap{
		"kind": MkProperty("plsChan"),
		"id":   MkProperty("whyDoIExist"),
	}
}

type MGSWithNoKind struct {
	S string
}

func (s *MGSWithNoKind) GetMeta(key string) (any, bool) {
	return nil, false
}

func (s *MGSWithNoKind) GetAllMeta() PropertyMap {
	return PropertyMap{"$kind": MkProperty("ohai")}
}

func (s *MGSWithNoKind) SetMeta(key string, val any) bool {
	return false
}

var _ MetaGetterSetter = (*MGSWithNoKind)(nil)

func TestKeyForObj(t *testing.T) {
	t.Parallel()

	ftt.Run("Test interface.KeyForObj", t, func(t *ftt.Test) {
		c := info.Set(context.Background(), fakeInfo{})
		fds := fakeDatastore{}
		c = SetRawFactory(c, fds.factory())

		k := MakeKey(c, "Hello", "world")

		t.Run("good", func(t *ftt.Test) {
			t.Run("struct containing $key", func(t *ftt.Test) {
				type keyStruct struct {
					Key *Key `gae:"$key"`
				}

				ks := &keyStruct{k}
				assert.Loosely(t, KeyForObj(c, ks), should.Equal(k))
			})

			t.Run("struct containing default $id and $kind", func(t *ftt.Test) {
				type idStruct struct {
					id  string `gae:"$id,wut"`
					knd string `gae:"$kind,SuperKind"`
				}

				assert.Loosely(t, KeyForObj(c, &idStruct{}).String(), should.Equal(`s~aid:ns:/SuperKind,"wut"`))
			})

			t.Run("struct containing $id and $parent", func(t *ftt.Test) {
				assert.Loosely(t, KeyForObj(c, &CommonStruct{ID: 4}).String(), should.Equal(`s~aid:ns:/CommonStruct,4`))

				assert.Loosely(t, KeyForObj(c, &CommonStruct{ID: 4, Parent: k}).String(), should.Equal(`s~aid:ns:/Hello,"world"/CommonStruct,4`))
			})

			t.Run("a propmap with $key", func(t *ftt.Test) {
				pm := PropertyMap{}
				assert.Loosely(t, pm.SetMeta("key", k), should.BeTrue)
				assert.Loosely(t, KeyForObj(c, pm).String(), should.Equal(`s~aid:ns:/Hello,"world"`))
			})

			t.Run("a propmap with $id, $kind, $parent", func(t *ftt.Test) {
				pm := PropertyMap{}
				assert.Loosely(t, pm.SetMeta("id", 100), should.BeTrue)
				assert.Loosely(t, pm.SetMeta("kind", "Sup"), should.BeTrue)
				assert.Loosely(t, KeyForObj(c, pm).String(), should.Equal(`s~aid:ns:/Sup,100`))

				assert.Loosely(t, pm.SetMeta("parent", k), should.BeTrue)
				assert.Loosely(t, KeyForObj(c, pm).String(), should.Equal(`s~aid:ns:/Hello,"world"/Sup,100`))
			})

			t.Run("a pls with $id, $parent", func(t *ftt.Test) {
				pls := GetPLS(&CommonStruct{ID: 1})
				assert.Loosely(t, KeyForObj(c, pls).String(), should.Equal(`s~aid:ns:/CommonStruct,1`))

				assert.Loosely(t, pls.SetMeta("parent", k), should.BeTrue)
				assert.Loosely(t, KeyForObj(c, pls).String(), should.Equal(`s~aid:ns:/Hello,"world"/CommonStruct,1`))
			})
		})

		t.Run("bad", func(t *ftt.Test) {
			t.Run("a propmap without $kind", func(t *ftt.Test) {
				pm := PropertyMap{}
				assert.Loosely(t, pm.SetMeta("id", 100), should.BeTrue)
				assert.Loosely(t, func() { KeyForObj(c, pm) }, should.Panic)
			})

			t.Run("a bad object", func(t *ftt.Test) {
				type BadObj struct {
					ID int64 `gae:"$id"`

					NonSerializableField complex64
				}

				assert.Loosely(t, func() { KeyForObj(c, &BadObj{ID: 1}) }, should.PanicLike(
					`field "NonSerializableField" has invalid type: complex64`))
			})
		})
	})
}

func TestPopulateKey(t *testing.T) {
	t.Parallel()

	ftt.Run("Test PopulateKey", t, func(t *ftt.Test) {
		kc := MkKeyContext("app", "namespace")
		k := kc.NewKey("kind", "", 1337, nil)

		t.Run("Can set the key of a common struct.", func(t *ftt.Test) {
			var cs CommonStruct

			assert.Loosely(t, PopulateKey(&cs, k), should.BeTrue)
			assert.Loosely(t, cs.ID, should.Equal(1337))
		})

		t.Run("Can set the parent key of a const id struct.", func(t *ftt.Test) {
			var s ConstIDStruct
			k2 := kc.NewKey("Bar", "bar", 0, k)
			PopulateKey(&s, k2)
			assert.Loosely(t, s.Parent, should.Resemble(k))
		})

		t.Run("Will not set the value of a singleton struct.", func(t *ftt.Test) {
			var ss SingletonStruct

			assert.Loosely(t, PopulateKey(&ss, k), should.BeFalse)
			assert.Loosely(t, ss.id, should.BeZero)
		})

		t.Run("Will panic when setting the key of a bad struct.", func(t *ftt.Test) {
			var bs badStruct

			assert.Loosely(t, func() { PopulateKey(&bs, k) }, should.Panic)
		})

		t.Run("Will panic when setting the key of a broken PLS struct.", func(t *ftt.Test) {
			var broken permaBad

			assert.Loosely(t, func() { PopulateKey(&broken, k) }, should.Panic)
		})
	})
}

func TestAllocateIDs(t *testing.T) {
	t.Parallel()

	ftt.Run("A testing environment", t, func(t *ftt.Test) {
		c := info.Set(context.Background(), fakeInfo{})
		fds := fakeDatastore{}
		c = SetRawFactory(c, fds.factory())

		t.Run("Testing AllocateIDs", func(t *ftt.Test) {
			t.Run("Will return nil if no entities are supplied.", func(t *ftt.Test) {
				assert.Loosely(t, AllocateIDs(c), should.BeNil)
			})

			t.Run("single struct", func(t *ftt.Test) {
				cs := CommonStruct{Value: 1}
				assert.Loosely(t, AllocateIDs(c, &cs), should.BeNil)
				assert.Loosely(t, cs.ID, should.Equal(1))
			})

			t.Run("struct slice", func(t *ftt.Test) {
				csSlice := []*CommonStruct{{Value: 1}, {Value: 2}}
				assert.Loosely(t, AllocateIDs(c, csSlice), should.BeNil)
				assert.Loosely(t, csSlice, should.Resemble([]*CommonStruct{{ID: 1, Value: 1}, {ID: 2, Value: 2}}))
			})

			t.Run("single key will fail", func(t *ftt.Test) {
				singleKey := MakeKey(c, "FooParent", "BarParent", "Foo", "Bar")
				assert.Loosely(t, func() { AllocateIDs(c, singleKey) }, should.PanicLike(
					"invalid input type (*datastore.Key): not a PLS, pointer-to-struct, or slice thereof"))
			})

			t.Run("key slice", func(t *ftt.Test) {
				k0 := MakeKey(c, "Foo", "Bar")
				k1 := MakeKey(c, "Baz", "Qux")
				keySlice := []*Key{k0, k1}
				assert.Loosely(t, AllocateIDs(c, keySlice), should.BeNil)
				assert.Loosely(t, keySlice[0].Equal(MakeKey(c, "Foo", 1)), should.BeTrue)
				assert.Loosely(t, keySlice[1].Equal(MakeKey(c, "Baz", 2)), should.BeTrue)

				// The original keys should not have changed.
				assert.Loosely(t, k0.Equal(MakeKey(c, "Foo", "Bar")), should.BeTrue)
				assert.Loosely(t, k1.Equal(MakeKey(c, "Baz", "Qux")), should.BeTrue)
			})

			t.Run("fail all key slice", func(t *ftt.Test) {
				keySlice := []*Key{MakeKey(c, "FailAll", "oops"), MakeKey(c, "Baz", "Qux")}
				assert.Loosely(t, AllocateIDs(c, keySlice), should.Equal(errFailAll))
				assert.Loosely(t, keySlice[0].StringID(), should.Equal("oops"))
				assert.Loosely(t, keySlice[1].StringID(), should.Equal("Qux"))
			})

			t.Run("fail key slice", func(t *ftt.Test) {
				keySlice := []*Key{MakeKey(c, "Fail", "oops"), MakeKey(c, "Baz", "Qux")}
				assert.Loosely(t, AllocateIDs(c, keySlice), should.ErrLike(
					errors.MultiError{errFail, nil}))
				assert.Loosely(t, keySlice[0].StringID(), should.Equal("oops"))
				assert.Loosely(t, keySlice[1].IntID(), should.Equal(2))
			})

			t.Run("vararg with errors", func(t *ftt.Test) {
				successSlice := []CommonStruct{{Value: 0}, {Value: 1}}
				failSlice := []FakePLS{{Kind: "Fail"}, {Value: 3}}
				emptySlice := []CommonStruct(nil)
				cs0 := CommonStruct{Value: 4}
				cs1 := FakePLS{Kind: "Fail", Value: 5}
				keySlice := []*Key{MakeKey(c, "Foo", "Bar"), MakeKey(c, "Baz", "Qux")}
				fpls := FakePLS{StringID: "ohai", Value: 6}

				err := AllocateIDs(c, successSlice, failSlice, emptySlice, &cs0, &cs1, keySlice, &fpls)
				assert.Loosely(t, err, should.ErrLike(errors.MultiError{
					nil, errors.MultiError{errFail, nil}, nil, nil, errFail, nil, nil},
				))
				assert.Loosely(t, successSlice[0].ID, should.Equal(1))
				assert.Loosely(t, successSlice[1].ID, should.Equal(2))
				assert.Loosely(t, failSlice[1].IntID, should.Equal(4))
				assert.Loosely(t, cs0.ID, should.Equal(5))
				assert.Loosely(t, keySlice[0].Equal(MakeKey(c, "Foo", 7)), should.BeTrue)
				assert.Loosely(t, keySlice[1].Equal(MakeKey(c, "Baz", 8)), should.BeTrue)
				assert.Loosely(t, fpls.IntID, should.Equal(9))
			})
		})
	})
}

func TestPut(t *testing.T) {
	t.Parallel()

	ftt.Run("A testing environment", t, func(t *ftt.Test) {
		c := info.Set(context.Background(), fakeInfo{})
		fds := fakeDatastore{}
		c = SetRawFactory(c, fds.factory())

		t.Run("Testing Put", func(t *ftt.Test) {
			t.Run("bad", func(t *ftt.Test) {
				t.Run("static can't serialize", func(t *ftt.Test) {
					bss := []badStruct{{}, {}}
					assert.Loosely(t, func() { Put(c, bss) }, should.PanicLike(
						`field "Compy" has invalid type`))
				})

				t.Run("static ptr can't serialize", func(t *ftt.Test) {
					bss := []*badStruct{{}, {}}
					assert.Loosely(t, func() { Put(c, bss) }, should.PanicLike(
						`field "Compy" has invalid type: complex64`))
				})

				t.Run("static bad type", func(t *ftt.Test) {
					assert.Loosely(t, func() { Put(c, 100) }, should.PanicLike(
						"invalid input type (int): not a PLS, pointer-to-struct, or slice thereof"))
				})

				t.Run("static bad type (slice of bad type)", func(t *ftt.Test) {
					assert.Loosely(t, func() { Put(c, []int{}) }, should.PanicLike(
						"invalid input type ([]int): not a PLS, pointer-to-struct, or slice thereof"))
				})

				t.Run("dynamic can't serialize", func(t *ftt.Test) {
					fplss := []FakePLS{{failSave: true}, {}}
					assert.Loosely(t, Put(c, fplss), should.ErrLike("FakePLS.Save"))
				})

				t.Run("can't get keys", func(t *ftt.Test) {
					fplss := []FakePLS{{failGetMeta: true}, {}}
					assert.Loosely(t, Put(c, fplss), should.ErrLike("unable to extract $kind"))
				})

				t.Run("get single error for RPC failure", func(t *ftt.Test) {
					fplss := []FakePLS{{Kind: "FailAll"}, {}}
					assert.Loosely(t, Put(c, fplss), should.Equal(errFailAll))
				})

				t.Run("get multi error for individual failures", func(t *ftt.Test) {
					fplss := []FakePLS{{}, {Kind: "Fail"}}
					assert.Loosely(t, Put(c, fplss), should.ErrLike(errors.MultiError{nil, errFail}))
				})

				t.Run("get with *Key is an error", func(t *ftt.Test) {
					assert.Loosely(t, func() { Get(c, &Key{}) }, should.PanicLike(
						"invalid input type (*datastore.Key): not a PLS, pointer-to-struct, or slice thereof"))
				})

				t.Run("struct with no $kind is an error", func(t *ftt.Test) {
					s := MGSWithNoKind{}
					assert.Loosely(t, Put(c, &s), should.ErrLike("unable to extract $kind"))
				})

				t.Run("struct with invalid but non-nil key is an error", func(t *ftt.Test) {
					type BadParent struct {
						ID     int64 `gae:"$id"`
						Parent *Key  `gae:"$parent"`
					}
					// having an Incomplete parent makes an invalid key
					bp := &BadParent{ID: 1, Parent: MakeKey(c, "Something", 0)}
					assert.Loosely(t, IsErrInvalidKey(Put(c, bp)), should.BeTrue)
				})

				t.Run("vararg with errors", func(t *ftt.Test) {
					successSlice := []CommonStruct{{Value: 0}, {Value: 1}}
					failSlice := []FakePLS{{Kind: "Fail"}, {Value: 3}}
					emptySlice := []CommonStruct(nil)
					cs0 := CommonStruct{Value: 4}
					failPLS := FakePLS{Kind: "Fail", Value: 5}
					fpls := FakePLS{StringID: "ohai", Value: 6}

					err := Put(c, successSlice, failSlice, emptySlice, &cs0, &failPLS, &fpls)
					assert.Loosely(t, err, should.ErrLike(errors.MultiError{
						nil, errors.MultiError{errFail, nil}, nil, nil, errFail, nil}))
					assert.Loosely(t, successSlice[0].ID, should.Equal(1))
					assert.Loosely(t, successSlice[1].ID, should.Equal(2))
					assert.Loosely(t, cs0.ID, should.Equal(5))
				})
			})

			t.Run("ok", func(t *ftt.Test) {
				t.Run("[]S", func(t *ftt.Test) {
					css := make([]CommonStruct, 7)
					for i := range css {
						if i == 4 {
							css[i].ID = 200
						}
						css[i].Value = int64(i)
					}
					assert.Loosely(t, Put(c, css), should.BeNil)
					for i, cs := range css {
						expect := int64(i + 1)
						if i == 4 {
							expect = 200
						}
						assert.Loosely(t, cs.ID, should.Equal(expect))
					}
				})

				t.Run("[]*S", func(t *ftt.Test) {
					css := make([]*CommonStruct, 7)
					for i := range css {
						css[i] = &CommonStruct{Value: int64(i)}
						if i == 4 {
							css[i].ID = 200
						}
					}
					assert.Loosely(t, Put(c, css), should.BeNil)
					for i, cs := range css {
						expect := int64(i + 1)
						if i == 4 {
							expect = 200
						}
						assert.Loosely(t, cs.ID, should.Equal(expect))
					}

					s := &CommonStruct{}
					assert.Loosely(t, Put(c, s), should.BeNil)
					assert.Loosely(t, s.ID, should.Equal(1))
				})

				t.Run("[]P", func(t *ftt.Test) {
					fplss := make([]FakePLS, 7)
					for i := range fplss {
						fplss[i].Value = int64(i)
						if i == 4 {
							fplss[i].IntID = int64(200)
						}
					}
					assert.Loosely(t, Put(c, fplss), should.BeNil)
					for i, fpls := range fplss {
						expect := int64(i + 1)
						if i == 4 {
							expect = 200
						}
						assert.Loosely(t, fpls.IntID, should.Equal(expect))
					}

					pm := PropertyMap{"Value": MkProperty(0), "$kind": MkPropertyNI("Pmap")}
					assert.Loosely(t, Put(c, pm), should.BeNil)
					assert.Loosely(t, KeyForObj(c, pm).IntID(), should.Equal(1))
				})

				t.Run("[]P (map)", func(t *ftt.Test) {
					pms := make([]PropertyMap, 7)
					for i := range pms {
						pms[i] = PropertyMap{
							"$kind": MkProperty("Pmap"),
							"Value": MkProperty(i),
						}
						if i == 4 {
							assert.Loosely(t, pms[i].SetMeta("id", int64(200)), should.BeTrue)
						}
					}
					assert.Loosely(t, Put(c, pms), should.BeNil)
					for i, pm := range pms {
						expect := int64(i + 1)
						if i == 4 {
							expect = 200
						}
						assert.Loosely(t, KeyForObj(c, pm).String(), should.Equal(fmt.Sprintf("s~aid:ns:/Pmap,%d", expect)))
					}
				})

				t.Run("[]*P", func(t *ftt.Test) {
					fplss := make([]*FakePLS, 7)
					for i := range fplss {
						fplss[i] = &FakePLS{Value: int64(i)}
						if i == 4 {
							fplss[i].IntID = int64(200)
						}
					}
					assert.Loosely(t, Put(c, fplss), should.BeNil)
					for i, fpls := range fplss {
						expect := int64(i + 1)
						if i == 4 {
							expect = 200
						}
						assert.Loosely(t, fpls.IntID, should.Equal(expect))
					}
				})

				t.Run("[]*P (map)", func(t *ftt.Test) {
					pms := make([]*PropertyMap, 7)
					for i := range pms {
						pms[i] = &PropertyMap{
							"$kind": MkProperty("Pmap"),
							"Value": MkProperty(i),
						}
						if i == 4 {
							assert.Loosely(t, pms[i].SetMeta("id", int64(200)), should.BeTrue)
						}
					}
					assert.Loosely(t, Put(c, pms), should.BeNil)
					for i, pm := range pms {
						expect := int64(i + 1)
						if i == 4 {
							expect = 200
						}
						assert.Loosely(t, KeyForObj(c, *pm).String(), should.Equal(fmt.Sprintf("s~aid:ns:/Pmap,%d", expect)))
					}
				})

				t.Run("[]I", func(t *ftt.Test) {
					ifs := []any{
						&CommonStruct{Value: 0},
						&FakePLS{Value: 1},
						PropertyMap{"Value": MkProperty(2), "$kind": MkPropertyNI("Pmap")},
						&PropertyMap{"Value": MkProperty(3), "$kind": MkPropertyNI("Pmap")},
					}
					assert.Loosely(t, Put(c, ifs), should.BeNil)
					for i := range ifs {
						switch i {
						case 0:
							assert.Loosely(t, ifs[i].(*CommonStruct).ID, should.Equal(1))
						case 1:
							fpls := ifs[i].(*FakePLS)
							assert.Loosely(t, fpls.IntID, should.Equal(2))
						case 2:
							assert.Loosely(t, KeyForObj(c, ifs[i].(PropertyMap)).String(), should.Equal("s~aid:ns:/Pmap,3"))
						case 3:
							assert.Loosely(t, KeyForObj(c, *ifs[i].(*PropertyMap)).String(), should.Equal("s~aid:ns:/Pmap,4"))
						}
					}
				})

				t.Run("vararg (flat)", func(t *ftt.Test) {
					sa := CommonStruct{
						Parent: MakeKey(c, "Foo", "foo"),
						Value:  0,
					}
					sb := PrivateStruct{
						Value: 1,
						_id:   1,
					}
					sc := PrivateStruct{
						Value: 2,
						_id:   0, // Invalid, but cannot assign.
					}

					err := Put(c, &sa, &sb, &sc)
					assert.Loosely(t, err, should.BeNil)

					assert.Loosely(t, sa.ID, should.Equal(1))
					assert.Loosely(t, sb._id, should.Equal(1))
					assert.Loosely(t, sc._id, should.BeZero) // Could not be set, private.
				})
			})
		})
	})
}

func TestExists(t *testing.T) {
	t.Parallel()

	ftt.Run("A testing environment", t, func(t *ftt.Test) {
		c := info.Set(context.Background(), fakeInfo{})
		fds := fakeDatastore{}
		c = SetRawFactory(c, fds.factory())

		k := MakeKey(c, "Hello", "world")

		t.Run("Exists", func(t *ftt.Test) {
			// Single key.
			er, err := Exists(c, k)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, er.All(), should.BeTrue)

			// Single key failure.
			_, err = Exists(c, MakeKey(c, "Fail", "boom"))
			assert.Loosely(t, err, should.Equal(errFail))

			// Single slice of keys.
			er, err = Exists(c, []*Key{k, MakeKey(c, "hello", "other")})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, er.All(), should.BeTrue)

			// Single slice of keys failure.
			er, err = Exists(c, []*Key{k, MakeKey(c, "Fail", "boom")})
			assert.Loosely(t, err, should.ErrLike(errors.MultiError{nil, errFail}))
			assert.Loosely(t, er.Get(0, 0), should.BeTrue)

			// Single key missing.
			er, err = Exists(c, MakeKey(c, "DNE", "nope"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, er.Any(), should.BeFalse)

			// Multi-arg keys with one missing.
			er, err = Exists(c, k, MakeKey(c, "DNE", "other"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, er.Get(0), should.BeTrue)
			assert.Loosely(t, er.Get(1), should.BeFalse)

			// Multi-arg keys with two missing.
			er, err = Exists(c, MakeKey(c, "DNE", "nope"), MakeKey(c, "DNE", "other"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, er.Any(), should.BeFalse)

			// Single struct pointer.
			er, err = Exists(c, &CommonStruct{ID: 1})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, er.All(), should.BeTrue)

			// Multi-arg mixed key/struct/slices.
			er, err = Exists(c,
				&CommonStruct{ID: 1},
				[]*CommonStruct(nil),
				[]*Key{MakeKey(c, "DNE", "nope"), MakeKey(c, "hello", "ohai")},
			)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, er.Get(0), should.BeTrue)
			assert.Loosely(t, er.Get(1), should.BeTrue)
			assert.Loosely(t, er.Get(2), should.BeFalse)
			assert.Loosely(t, er.Get(2, 0), should.BeFalse)
			assert.Loosely(t, er.Get(2, 1), should.BeTrue)
		})
	})
}

func TestDelete(t *testing.T) {
	t.Parallel()

	ftt.Run("A testing environment", t, func(t *ftt.Test) {
		c := info.Set(context.Background(), fakeInfo{})
		fds := fakeDatastore{}
		c = SetRawFactory(c, fds.factory())

		t.Run("Testing Delete", func(t *ftt.Test) {
			t.Run("bad", func(t *ftt.Test) {
				t.Run("get single error for RPC failure", func(t *ftt.Test) {
					keys := []*Key{
						MakeKey(c, "s~aid", "ns", "FailAll", 1),
						MakeKey(c, "s~aid", "ns", "Ok", 1),
					}
					assert.Loosely(t, Delete(c, keys), should.Equal(errFailAll))
				})

				t.Run("get multi error for individual failure", func(t *ftt.Test) {
					keys := []*Key{
						MakeKey(c, "Ok", 1),
						MakeKey(c, "Fail", 2),
					}
					assert.Loosely(t, Delete(c, keys), should.ErrLike(errors.MultiError{
						nil, errFail,
					}))
				})

				t.Run("put with non-modifyable type is an error", func(t *ftt.Test) {
					cs := CommonStruct{}
					assert.Loosely(t, func() { Put(c, cs) }, should.PanicLike(
						"invalid input type (datastore.CommonStruct): not a pointer"))
				})

				t.Run("get single error when deleting a single", func(t *ftt.Test) {
					k := MakeKey(c, "Fail", 1)
					assert.Loosely(t, Delete(c, k), should.Equal(errFail))
				})
			})

			t.Run("good", func(t *ftt.Test) {
				// Single struct pointer.
				assert.Loosely(t, Delete(c, &CommonStruct{ID: 1}), should.BeNil)

				// Single key.
				assert.Loosely(t, Delete(c, MakeKey(c, "hello", "ohai")), should.BeNil)

				// Single struct DNE.
				assert.Loosely(t, Delete(c, &CommonStruct{ID: noSuchEntityID}), should.Equal(ErrNoSuchEntity))

				// Single key DNE.
				assert.Loosely(t, Delete(c, MakeKey(c, "DNE", "nope")), should.Equal(ErrNoSuchEntity))

				// Mixed key/struct/slices.
				err := Delete(c,
					&CommonStruct{ID: 1},
					[]*Key{MakeKey(c, "hello", "ohai"), MakeKey(c, "DNE", "nope")},
				)
				assert.That(t, err, should.ErrLike(errors.MultiError{
					nil,
					errors.MultiError{nil, ErrNoSuchEntity},
				}))
			})
		})
	})
}

func TestGet(t *testing.T) {
	t.Parallel()

	ftt.Run("A testing environment", t, func(t *ftt.Test) {
		c := info.Set(context.Background(), fakeInfo{})
		fds := fakeDatastore{}
		c = SetRawFactory(c, fds.factory())

		t.Run("Testing Get", func(t *ftt.Test) {
			t.Run("bad", func(t *ftt.Test) {
				t.Run("static can't serialize", func(t *ftt.Test) {
					toGet := []badStruct{{}, {}}
					assert.Loosely(t, func() { Get(c, toGet) }, should.PanicLike(
						`field "Compy" has invalid type: complex64`))
				})

				t.Run("can't get keys", func(t *ftt.Test) {
					fplss := []FakePLS{{failGetMeta: true}, {}}
					assert.Loosely(t, Get(c, fplss), should.ErrLike("unable to extract $kind"))
				})

				t.Run("get single error for RPC failure", func(t *ftt.Test) {
					fplss := []FakePLS{
						{IntID: 1, Kind: "FailAll"},
						{IntID: 2},
					}
					assert.Loosely(t, Get(c, fplss), should.Equal(errFailAll))
				})

				t.Run("get multi error for individual failures", func(t *ftt.Test) {
					fplss := []FakePLS{{IntID: 1}, {IntID: 2, Kind: "Fail"}}
					assert.Loosely(t, Get(c, fplss), should.ErrLike(
						errors.MultiError{nil, errFail}))
				})

				t.Run("get with non-modifiable type is an error", func(t *ftt.Test) {
					cs := CommonStruct{}
					assert.Loosely(t, func() { Get(c, cs) }, should.PanicLike(
						"invalid input type (datastore.CommonStruct): not a pointer"))
				})

				t.Run("get with nil is an error", func(t *ftt.Test) {
					assert.Loosely(t, func() { Get(c, nil) }, should.PanicLike(
						"cannot use nil as single argument"))
				})

				t.Run("get with typed nil is an error", func(t *ftt.Test) {
					assert.Loosely(t, func() { Get(c, (*CommonStruct)(nil)) }, should.PanicLike(
						"cannot use typed nil as single argument"))
				})

				t.Run("get with nil inside a slice is an error", func(t *ftt.Test) {
					assert.Loosely(t, func() { Get(c, []any{&CommonStruct{}, nil}) }, should.PanicLike(
						"invalid input slice: has nil at index 1"))
				})

				t.Run("get with typed nil inside a slice is an error", func(t *ftt.Test) {
					assert.Loosely(t, func() { Get(c, []any{&CommonStruct{}, (*CommonStruct)(nil)}) }, should.PanicLike(
						"invalid input slice: has nil at index 1"))
				})

				t.Run("get with nil inside a struct slice is an error", func(t *ftt.Test) {
					assert.Loosely(t, func() { _ = Get(c, []*CommonStruct{{}, nil}) }, should.PanicLike(
						"invalid input slice: has nil at index 1"))
				})

				t.Run("get with ptr-to-nonstruct is an error", func(t *ftt.Test) {
					val := 100
					assert.Loosely(t, func() { Get(c, &val) }, should.PanicLike(
						"invalid input type (*int): not a PLS, pointer-to-struct, or slice thereof"))
				})

				t.Run("failure to save metadata is no problem though", func(t *ftt.Test) {
					// It just won't save the key
					cs := &FakePLS{IntID: 10, failSetMeta: true}
					assert.Loosely(t, Get(c, cs), should.BeNil)
				})

				t.Run("vararg with errors", func(t *ftt.Test) {
					successSlice := []CommonStruct{{ID: 1}, {ID: 2}}
					failSlice := []CommonStruct{{ID: noSuchEntityID}, {ID: 3}}
					emptySlice := []CommonStruct(nil)
					cs0 := CommonStruct{ID: 4}
					failPLS := CommonStruct{ID: noSuchEntityID}
					fpls := FakePLS{StringID: "ohai"}

					err := Get(c, successSlice, failSlice, emptySlice, &cs0, &failPLS, &fpls)
					assert.Loosely(t, err, should.ErrLike(errors.MultiError{
						nil, errors.MultiError{ErrNoSuchEntity, nil}, nil, nil, ErrNoSuchEntity, nil}))
					assert.Loosely(t, successSlice[0].Value, should.Equal(1))
					assert.Loosely(t, successSlice[1].Value, should.Equal(2))
					assert.Loosely(t, cs0.Value, should.Equal(5))
					assert.Loosely(t, fpls.Value, should.Equal(7))
				})

				t.Run("mix of invalid key, type, and doesn't exist", func(t *ftt.Test) {
					successSlice := []CommonStruct{{ID: 1}, {ID: 2}}
					type badKind struct {
						ID    int64  `gae:"$id"`
						Kind  string `gae:"$kind"`
						Value string
					}
					badPMSlice := []any{
						&badKind{ID: 1},
						&FakePLS{IntID: 1, failLoad: true},
					}
					type idStruct struct {
						ID    string `gae:"$id"`
						Value string
					}
					badKeySlice := []any{&idStruct{Value: "hi"}, &CommonStruct{}}

					err := Get(c, successSlice, badPMSlice, badKeySlice)
					assert.Loosely(t, err, should.HaveType[errors.MultiError])

					merr := err.(errors.MultiError)
					assert.Loosely(t, merr, should.HaveLength(3))

					assert.Loosely(t, merr[0], should.BeNil)
					assert.Loosely(t, successSlice[0], should.Resemble(CommonStruct{ID: 1, Value: 1}))
					assert.Loosely(t, successSlice[1], should.Resemble(CommonStruct{ID: 2, Value: 2}))

					assert.Loosely(t, merr[1], should.HaveType[errors.MultiError])
					merr1 := merr[1].(errors.MultiError)
					assert.Loosely(t, merr1, should.HaveLength(2))
					assert.Loosely(t, merr1[0], should.ErrLike("unable to extract $kind"))
					assert.Loosely(t, merr1[1], should.ErrLike("FakePLS.Load"))

					assert.Loosely(t, merr[2], should.HaveType[errors.MultiError])
					merr2 := merr[2].(errors.MultiError)
					assert.Loosely(t, merr2, should.HaveLength(2))
					assert.Loosely(t, merr2[0], should.ErrLike("invalid key"))
					assert.Loosely(t, merr2[1], should.ErrLike("invalid key"))
				})
			})

			t.Run("ok", func(t *ftt.Test) {
				t.Run("Get", func(t *ftt.Test) {
					cs := &CommonStruct{ID: 1}
					assert.Loosely(t, Get(c, cs), should.BeNil)
					assert.Loosely(t, cs.Value, should.Equal(1))
				})

				t.Run("Raw access too", func(t *ftt.Test) {
					rds := Raw(c)
					keys := []*Key{MakeKey(c, "Kind", 1)}
					assert.Loosely(t, rds.GetMulti(keys, nil, func(_ int, pm PropertyMap, err error) {
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, pm.Slice("Value")[0].Value(), should.Equal(1))
					}), should.BeNil)
				})

				t.Run("but general failure to save is fine on a Get", func(t *ftt.Test) {
					cs := &FakePLS{failSave: true, IntID: 7}
					assert.Loosely(t, Get(c, cs), should.BeNil)
				})

				t.Run("vararg", func(t *ftt.Test) {
					successSlice := []CommonStruct{{ID: 1}, {ID: 2}}
					cs := CommonStruct{ID: 3}

					err := Get(c, successSlice, &cs)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, successSlice[0].Value, should.Equal(1))
					assert.Loosely(t, successSlice[1].Value, should.Equal(2))
					assert.Loosely(t, cs.Value, should.Equal(3))
				})
			})
		})
	})
}

func TestGetAll(t *testing.T) {
	t.Parallel()

	ftt.Run("Test GetAll", t, func(t *ftt.Test) {
		c := info.Set(context.Background(), fakeInfo{})
		fds := fakeDatastore{}
		c = SetRawFactory(c, fds.factory())

		fds.entities = 5
		q := NewQuery("")

		t.Run("bad", func(t *ftt.Test) {
			t.Run("nil target", func(t *ftt.Test) {
				assert.Loosely(t, func() { GetAll(c, q, (*[]PropertyMap)(nil)) }, should.PanicLike(
					"invalid GetAll dst: <nil>"))
			})

			t.Run("bad type", func(t *ftt.Test) {
				output := 100
				assert.Loosely(t, func() { GetAll(c, q, &output) }, should.PanicLike(
					"invalid argument type: expected slice, got int"))
			})

			t.Run("bad type (non pointer)", func(t *ftt.Test) {
				assert.Loosely(t, func() { GetAll(c, q, "moo") }, should.PanicLike(
					"invalid GetAll dst: must have a ptr-to-slice"))
			})

			t.Run("bad type (underspecified)", func(t *ftt.Test) {
				output := []PropertyLoadSaver(nil)
				assert.Loosely(t, func() { GetAll(c, q, &output) }, should.PanicLike(
					"invalid GetAll dst (non-concrete element type): *[]datastore.PropertyLoadSaver"))
			})
		})

		t.Run("ok", func(t *ftt.Test) {
			t.Run("*[]S", func(t *ftt.Test) {
				output := []CommonStruct(nil)
				assert.Loosely(t, GetAll(c, q, &output), should.BeNil)
				assert.Loosely(t, len(output), should.Equal(5))
				for i, o := range output {
					assert.Loosely(t, o.ID, should.Equal(i+1))
					assert.Loosely(t, o.Value, should.Equal(i))
				}
			})

			t.Run("*[]*S", func(t *ftt.Test) {
				output := []*CommonStruct(nil)
				assert.Loosely(t, GetAll(c, q, &output), should.BeNil)
				assert.Loosely(t, len(output), should.Equal(5))
				for i, o := range output {
					assert.Loosely(t, o.ID, should.Equal(i+1))
					assert.Loosely(t, o.Value, should.Equal(i))
				}
			})

			t.Run("*[]P", func(t *ftt.Test) {
				output := []FakePLS(nil)
				assert.Loosely(t, GetAll(c, q, &output), should.BeNil)
				assert.Loosely(t, len(output), should.Equal(5))
				for i, o := range output {
					assert.Loosely(t, o.gotLoaded, should.BeTrue)
					assert.Loosely(t, o.IntID, should.Equal(i+1))
					assert.Loosely(t, o.Value, should.Equal(i))
				}
			})

			t.Run("*[]P (map)", func(t *ftt.Test) {
				output := []PropertyMap(nil)
				assert.Loosely(t, GetAll(c, q, &output), should.BeNil)
				assert.Loosely(t, len(output), should.Equal(5))
				for i, o := range output {
					k, ok := o.GetMeta("key")
					assert.Loosely(t, ok, should.BeTrue)
					assert.Loosely(t, k.(*Key).IntID(), should.Equal(i+1))
					assert.Loosely(t, o.Slice("Value")[0].Value().(int64), should.Equal(i))
				}
			})

			t.Run("*[]P (chan)", func(t *ftt.Test) {
				output := []plsChan(nil)
				assert.Loosely(t, GetAll(c, q, &output), should.BeNil)
				assert.Loosely(t, output, should.HaveLength(5))
				for _, o := range output {
					assert.Loosely(t, KeyForObj(c, o).StringID(), should.Equal("whyDoIExist"))
				}
			})

			t.Run("*[]*P", func(t *ftt.Test) {
				output := []*FakePLS(nil)
				assert.Loosely(t, GetAll(c, q, &output), should.BeNil)
				assert.Loosely(t, len(output), should.Equal(5))
				for i, o := range output {
					assert.Loosely(t, o.gotLoaded, should.BeTrue)
					assert.Loosely(t, o.IntID, should.Equal(i+1))
					assert.Loosely(t, o.Value, should.Equal(i))
				}
			})

			t.Run("*[]*P (map)", func(t *ftt.Test) {
				output := []*PropertyMap(nil)
				assert.Loosely(t, GetAll(c, q, &output), should.BeNil)
				assert.Loosely(t, len(output), should.Equal(5))
				for i, op := range output {
					o := *op
					k, ok := o.GetMeta("key")
					assert.Loosely(t, ok, should.BeTrue)
					assert.Loosely(t, k.(*Key).IntID(), should.Equal(i+1))
					assert.Loosely(t, o.Slice("Value")[0].Value().(int64), should.Equal(i))
				}
			})

			t.Run("*[]*P (chan)", func(t *ftt.Test) {
				output := []*plsChan(nil)
				assert.Loosely(t, GetAll(c, q, &output), should.BeNil)
				assert.Loosely(t, output, should.HaveLength(5))
				for _, o := range output {
					assert.Loosely(t, KeyForObj(c, o).StringID(), should.Equal("whyDoIExist"))
				}
			})

			t.Run("*[]*Key", func(t *ftt.Test) {
				output := []*Key(nil)
				assert.Loosely(t, GetAll(c, q, &output), should.BeNil)
				assert.Loosely(t, len(output), should.Equal(5))
				for i, k := range output {
					assert.Loosely(t, k.IntID(), should.Equal(i+1))
				}
			})

		})
	})
}

func TestRun(t *testing.T) {
	t.Parallel()

	ftt.Run("Test Run", t, func(t *ftt.Test) {
		c := info.Set(context.Background(), fakeInfo{})
		fds := fakeDatastore{}
		c = SetRawFactory(c, fds.factory())

		fds.entities = 5
		q := NewQuery("kind")

		t.Run("bad", func(t *ftt.Test) {
			assertBadTypePanics := func(cb any) {
				assert.Loosely(t, func() { Run(c, q, cb) }, should.PanicLike(
					"cb does not match the required callback signature"))
			}

			t.Run("not a function", func(t *ftt.Test) {
				assertBadTypePanics("I am a potato")
			})

			t.Run("nil", func(t *ftt.Test) {
				assertBadTypePanics(nil)
			})

			t.Run("interface", func(t *ftt.Test) {
				assertBadTypePanics(func(pls PropertyLoadSaver) {})
			})

			t.Run("bad proto type", func(t *ftt.Test) {
				cb := func(v int) {
					panic("never here!")
				}
				assert.Loosely(t, func() { Run(c, q, cb) }, should.PanicLike(
					"invalid argument type: int is not a PLS or pointer-to-struct"))
			})

			t.Run("wrong # args", func(t *ftt.Test) {
				assertBadTypePanics(func(v CommonStruct, _ CursorCB, _ int) {
					panic("never here!")
				})
			})

			t.Run("wrong ret type", func(t *ftt.Test) {
				assertBadTypePanics(func(v CommonStruct) bool {
					panic("never here!")
				})
			})

			t.Run("wrong # rets", func(t *ftt.Test) {
				assertBadTypePanics(func(v CommonStruct) (int, error) {
					panic("never here!")
				})
			})

			t.Run("bad 2nd arg", func(t *ftt.Test) {
				assertBadTypePanics(func(v CommonStruct, _ Cursor) error {
					panic("never here!")
				})
			})

			t.Run("early abort on error", func(t *ftt.Test) {
				q = q.Eq("@err_single", "Query fail").Eq("@err_single_idx", 3)
				i := 0
				assert.Loosely(t, Run(c, q, func(c CommonStruct) {
					i++
				}), should.ErrLike("Query fail"))
				assert.Loosely(t, i, should.Equal(3))
			})

			t.Run("return error on serialization failure", func(t *ftt.Test) {
				assert.Loosely(t, Run(c, q, func(_ permaBad) {
					panic("never here")
				}).Error(), should.Equal("permaBad"))
			})
		})

		t.Run("ok", func(t *ftt.Test) {
			t.Run("can return error to stop", func(t *ftt.Test) {
				i := 0
				assert.Loosely(t, Run(c, q, func(c CommonStruct) error {
					i++
					return Stop
				}), should.BeNil)
				assert.Loosely(t, i, should.Equal(1))

				i = 0
				assert.Loosely(t, Run(c, q, func(c CommonStruct, _ CursorCB) error {
					i++
					return fmt.Errorf("my error")
				}), should.ErrLike("my error"))
				assert.Loosely(t, i, should.Equal(1))
			})

			t.Run("Can optionally get cursor function", func(t *ftt.Test) {
				i := 0
				assert.Loosely(t, Run(c, q, func(c CommonStruct, ccb CursorCB) {
					i++
					curs, err := ccb()
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, curs.String(), should.Equal(fakeCursor(i).String()))
				}), should.BeNil)
				assert.Loosely(t, i, should.Equal(5))
			})

			t.Run("*S", func(t *ftt.Test) {
				i := 0
				assert.Loosely(t, Run(c, q, func(cs *CommonStruct) {
					assert.Loosely(t, cs.ID, should.Equal(i+1))
					assert.Loosely(t, cs.Value, should.Equal(i))
					i++
				}), should.BeNil)
			})

			t.Run("*P", func(t *ftt.Test) {
				fds.keyForResult = func(i int32, kctx KeyContext) *Key {
					if i == 10 {
						return kctx.MakeKey("Kind", "eleven")
					}
					return kctx.MakeKey("Kind", i+1)
				}

				i := 0
				assert.Loosely(t, Run(c, q.Limit(12), func(fpls *FakePLS) {
					assert.Loosely(t, fpls.gotLoaded, should.BeTrue)
					if i == 10 {
						assert.Loosely(t, fpls.StringID, should.Equal("eleven"))
					} else {
						assert.Loosely(t, fpls.IntID, should.Equal(i+1))
					}
					assert.Loosely(t, fpls.Value, should.Equal(i))
					i++
				}), should.BeNil)
			})

			t.Run("*P (map)", func(t *ftt.Test) {
				i := 0
				assert.Loosely(t, Run(c, q, func(pm *PropertyMap) {
					k, ok := pm.GetMeta("key")
					assert.Loosely(t, ok, should.BeTrue)
					assert.Loosely(t, k.(*Key).IntID(), should.Equal(i+1))
					assert.Loosely(t, (*pm).Slice("Value")[0].Value(), should.Equal(i))
					i++
				}), should.BeNil)
			})

			t.Run("*P (chan)", func(t *ftt.Test) {
				assert.Loosely(t, Run(c, q, func(ch *plsChan) {
					assert.Loosely(t, KeyForObj(c, ch).StringID(), should.Equal("whyDoIExist"))
				}), should.BeNil)
			})

			t.Run("S", func(t *ftt.Test) {
				i := 0
				assert.Loosely(t, Run(c, q, func(cs CommonStruct) {
					assert.Loosely(t, cs.ID, should.Equal(i+1))
					assert.Loosely(t, cs.Value, should.Equal(i))
					i++
				}), should.BeNil)
			})

			t.Run("P", func(t *ftt.Test) {
				i := 0
				assert.Loosely(t, Run(c, q, func(fpls FakePLS) {
					assert.Loosely(t, fpls.gotLoaded, should.BeTrue)
					assert.Loosely(t, fpls.IntID, should.Equal(i+1))
					assert.Loosely(t, fpls.Value, should.Equal(i))
					i++
				}), should.BeNil)
			})

			t.Run("P (map)", func(t *ftt.Test) {
				i := 0
				assert.Loosely(t, Run(c, q, func(pm PropertyMap) {
					k, ok := pm.GetMeta("key")
					assert.Loosely(t, ok, should.BeTrue)
					assert.Loosely(t, k.(*Key).IntID(), should.Equal(i+1))
					assert.Loosely(t, pm.Slice("Value")[0].Value(), should.Equal(i))
					i++
				}), should.BeNil)
			})

			t.Run("P (chan)", func(t *ftt.Test) {
				assert.Loosely(t, Run(c, q, func(ch plsChan) {
					assert.Loosely(t, KeyForObj(c, ch).StringID(), should.Equal("whyDoIExist"))
				}), should.BeNil)
			})

			t.Run("Key", func(t *ftt.Test) {
				i := 0
				assert.Loosely(t, Run(c, q, func(k *Key) {
					assert.Loosely(t, k.IntID(), should.Equal(i+1))
					i++
				}), should.BeNil)
			})

		})
	})
}

type fixedDataDatastore struct {
	RawInterface

	data map[string]PropertyMap
}

func (d *fixedDataDatastore) GetMulti(keys []*Key, _ MultiMetaGetter, cb GetMultiCB) error {
	for i, k := range keys {
		data, ok := d.data[k.String()]
		if ok {
			cb(i, data, nil)
		} else {
			cb(i, nil, ErrNoSuchEntity)
		}
	}
	return nil
}

func (d *fixedDataDatastore) PutMulti(keys []*Key, vals []PropertyMap, cb NewKeyCB) error {
	if d.data == nil {
		d.data = make(map[string]PropertyMap, len(keys))
	}
	for i, k := range keys {
		if k.IsIncomplete() {
			panic("key is incomplete, don't do that.")
		}
		d.data[k.String()], _ = vals[i].Save(false)
		cb(i, k, nil)
	}
	return nil
}

func (d *fixedDataDatastore) Constraints() Constraints { return Constraints{} }

func TestSchemaChange(t *testing.T) {
	t.Parallel()

	ftt.Run("Test changing schemas", t, func(t *ftt.Test) {
		fds := fixedDataDatastore{}
		c := info.Set(context.Background(), fakeInfo{})
		c = SetRaw(c, &fds)

		t.Run("Can add fields", func(t *ftt.Test) {
			initial := PropertyMap{
				"$key": mpNI(MakeKey(c, "Val", 10)),
				"Val":  mp(100),
			}
			assert.Loosely(t, Put(c, initial), should.BeNil)

			type Val struct {
				ID int64 `gae:"$id"`

				Val    int64
				TwoVal int64 // whoa, TWO vals! amazing
			}
			tv := &Val{ID: 10, TwoVal: 2}
			assert.Loosely(t, Get(c, tv), should.BeNil)
			assert.Loosely(t, tv, should.Resemble(&Val{ID: 10, Val: 100, TwoVal: 0 /*was reset*/}))
		})

		t.Run("Removing fields", func(t *ftt.Test) {
			initial := PropertyMap{
				"$key":   mpNI(MakeKey(c, "Val", 10)),
				"Val":    mp(100),
				"TwoVal": mp(200),
			}
			assert.Loosely(t, Put(c, initial), should.BeNil)

			t.Run("is normally an error", func(t *ftt.Test) {
				type Val struct {
					ID int64 `gae:"$id"`

					Val int64
				}
				tv := &Val{ID: 10}
				assert.Loosely(t, Get(c, tv), should.ErrLike(
					`gae: cannot load field "TwoVal" into a "datastore.Val`))
				assert.Loosely(t, tv, should.Resemble(&Val{ID: 10, Val: 100}))
			})

			t.Run("Unless you have an ,extra field!", func(t *ftt.Test) {
				type Val struct {
					ID int64 `gae:"$id"`

					Val   int64
					Extra PropertyMap `gae:",extra"`
				}
				tv := &Val{ID: 10}
				assert.Loosely(t, Get(c, tv), should.BeNil)
				assert.Loosely(t, tv, should.Resemble(&Val{
					ID:  10,
					Val: 100,
					Extra: PropertyMap{
						"TwoVal": PropertySlice{mp(200)},
					},
				}))
			})
		})

		t.Run("Can round-trip extra fields", func(t *ftt.Test) {
			type Expando struct {
				ID int64 `gae:"$id"`

				Something int
				Extra     PropertyMap `gae:",extra"`
			}
			ex := &Expando{10, 17, PropertyMap{
				"Hello": mp("Hello"),
				"World": mp(true),
			}}
			assert.Loosely(t, Put(c, ex), should.BeNil)

			ex = &Expando{ID: 10}
			assert.Loosely(t, Get(c, ex), should.BeNil)
			assert.Loosely(t, ex, should.Resemble(&Expando{
				ID:        10,
				Something: 17,
				Extra: PropertyMap{
					"Hello": PropertySlice{mp("Hello")},
					"World": PropertySlice{mp(true)},
				},
			}))
		})

		t.Run("Can read-but-not-write", func(t *ftt.Test) {
			initial := PropertyMap{
				"$key":   mpNI(MakeKey(c, "Convert", 10)),
				"Val":    mp(100),
				"TwoVal": mp(200),
			}
			assert.Loosely(t, Put(c, initial), should.BeNil)
			type Convert struct {
				ID int64 `gae:"$id"`

				Val    int64
				NewVal int64
				Extra  PropertyMap `gae:"-,extra"`
			}
			cnv := &Convert{ID: 10}
			assert.Loosely(t, Get(c, cnv), should.BeNil)
			assert.Loosely(t, cnv, should.Resemble(&Convert{
				ID: 10, Val: 100, NewVal: 0, Extra: PropertyMap{"TwoVal": PropertySlice{mp(200)}},
			}))
			cnv.NewVal = cnv.Extra.Slice("TwoVal")[0].Value().(int64)
			assert.Loosely(t, Put(c, cnv), should.BeNil)

			cnv = &Convert{ID: 10}
			assert.Loosely(t, Get(c, cnv), should.BeNil)
			assert.Loosely(t, cnv, should.Resemble(&Convert{
				ID: 10, Val: 100, NewVal: 200, Extra: nil,
			}))
		})

		t.Run("Can black hole", func(t *ftt.Test) {
			initial := PropertyMap{
				"$key":   mpNI(MakeKey(c, "BlackHole", 10)),
				"Val":    mp(100),
				"TwoVal": mp(200),
			}
			assert.Loosely(t, Put(c, initial), should.BeNil)
			type BlackHole struct {
				ID int64 `gae:"$id"`

				NewStuff  string
				blackHole PropertyMap `gae:"-,extra"`
			}
			b := &BlackHole{ID: 10}
			assert.Loosely(t, Get(c, b), should.BeNil)
			assert.Loosely(t, b, should.Resemble(&BlackHole{ID: 10}))
		})

		t.Run("Can change field types", func(t *ftt.Test) {
			initial := PropertyMap{
				"$key": mpNI(MakeKey(c, "IntChange", 10)),
				"Val":  mp(100),
			}
			assert.Loosely(t, Put(c, initial), should.BeNil)

			type IntChange struct {
				ID    int64 `gae:"$id"`
				Val   string
				Extra PropertyMap `gae:"-,extra"`
			}
			i := &IntChange{ID: 10}
			assert.Loosely(t, Get(c, i), should.BeNil)
			assert.Loosely(t, i, should.Resemble(&IntChange{ID: 10, Extra: PropertyMap{"Val": PropertySlice{mp(100)}}}))
			i.Val = fmt.Sprint(i.Extra.Slice("Val")[0].Value())
			assert.Loosely(t, Put(c, i), should.BeNil)

			i = &IntChange{ID: 10}
			assert.Loosely(t, Get(c, i), should.BeNil)
			assert.Loosely(t, i, should.Resemble(&IntChange{ID: 10, Val: "100"}))
		})

		t.Run("Native fields have priority over Extra fields", func(t *ftt.Test) {
			type Dup struct {
				ID    int64 `gae:"$id"`
				Val   int64
				Extra PropertyMap `gae:",extra"`
			}
			d := &Dup{ID: 10, Val: 100, Extra: PropertyMap{
				"Val":   PropertySlice{mp(200)},
				"Other": PropertySlice{mp("other")},
			}}
			assert.Loosely(t, Put(c, d), should.BeNil)

			d = &Dup{ID: 10}
			assert.Loosely(t, Get(c, d), should.BeNil)
			assert.Loosely(t, d, should.Resemble(&Dup{
				ID: 10, Val: 100, Extra: PropertyMap{"Other": PropertySlice{mp("other")}},
			}))
		})

		t.Run("Can change repeated field to non-repeating field", func(t *ftt.Test) {
			initial := PropertyMap{
				"$key": mpNI(MakeKey(c, "NonRepeating", 10)),
				"Val":  PropertySlice{mp(100), mp(200), mp(400)},
			}
			assert.Loosely(t, Put(c, initial), should.BeNil)

			type NonRepeating struct {
				ID    int64 `gae:"$id"`
				Val   int64
				Extra PropertyMap `gae:",extra"`
			}
			n := &NonRepeating{ID: 10}
			assert.Loosely(t, Get(c, n), should.BeNil)
			assert.Loosely(t, n, should.Resemble(&NonRepeating{
				ID: 10, Val: 0, Extra: PropertyMap{
					"Val": PropertySlice{mp(100), mp(200), mp(400)},
				},
			}))
		})

		t.Run("Deals correctly with recursive types", func(t *ftt.Test) {
			initial := PropertyMap{
				"$key": mpNI(MakeKey(c, "Outer", 10)),
				"I.A":  PropertySlice{mp(1), mp(2), mp(4)},
				"I.B":  PropertySlice{mp(10), mp(20), mp(40)},
				"I.C":  PropertySlice{mp(100), mp(200), mp(400)},
			}
			assert.Loosely(t, Put(c, initial), should.BeNil)
			type Inner struct {
				A int64
				B int64
			}
			type Outer struct {
				ID int64 `gae:"$id"`

				I     []Inner
				Extra PropertyMap `gae:",extra"`
			}
			o := &Outer{ID: 10}
			assert.Loosely(t, Get(c, o), should.BeNil)
			assert.Loosely(t, o, should.Resemble(&Outer{
				ID: 10,
				I: []Inner{
					{1, 10},
					{2, 20},
					{4, 40},
				},
				Extra: PropertyMap{
					"I.C": PropertySlice{mp(100), mp(200), mp(400)},
				},
			}))
		})

		t.Run("Problems", func(t *ftt.Test) {
			t.Run("multiple extra fields", func(t *ftt.Test) {
				type Bad struct {
					A PropertyMap `gae:",extra"`
					B PropertyMap `gae:",extra"`
				}
				assert.Loosely(t, func() { GetPLS(&Bad{}) }, should.PanicLike(
					"multiple fields tagged as 'extra'"))
			})

			t.Run("extra field with name", func(t *ftt.Test) {
				type Bad struct {
					A PropertyMap `gae:"wut,extra"`
				}
				assert.Loosely(t, func() { GetPLS(&Bad{}) }, should.PanicLike(
					"struct 'extra' field has invalid name wut"))
			})

			t.Run("extra field with bad type", func(t *ftt.Test) {
				type Bad struct {
					A int64 `gae:",extra"`
				}
				assert.Loosely(t, func() { GetPLS(&Bad{}) }, should.PanicLike(
					"struct 'extra' field has invalid type int64"))
			})
		})
	})
}

func TestParseIndexYAML(t *testing.T) {
	t.Parallel()

	ftt.Run("parses properly formatted YAML", t, func(t *ftt.Test) {
		yaml := `
indexes:

- kind: Cat
  ancestor: no
  properties:
  - name: name
  - name: age
    direction: desc

- kind: Cat
  properties:
  - name: name
    direction: asc
  - name: whiskers
    direction: desc

- kind: Store
  ancestor: yes
  properties:
  - name: business
    direction: asc
  - name: owner
    direction: asc
`
		ids, err := ParseIndexYAML(bytes.NewBuffer([]byte(yaml)))
		assert.Loosely(t, err, should.BeNil)

		expected := []*IndexDefinition{
			{
				Kind:     "Cat",
				Ancestor: false,
				SortBy: []IndexColumn{
					{
						Property:   "name",
						Descending: false,
					},
					{
						Property:   "age",
						Descending: true,
					},
				},
			},
			{
				Kind:     "Cat",
				Ancestor: false,
				SortBy: []IndexColumn{
					{
						Property:   "name",
						Descending: false,
					},
					{
						Property:   "whiskers",
						Descending: true,
					},
				},
			},
			{
				Kind:     "Store",
				Ancestor: true,
				SortBy: []IndexColumn{
					{
						Property:   "business",
						Descending: false,
					},
					{
						Property:   "owner",
						Descending: false,
					},
				},
			},
		}
		assert.Loosely(t, ids, should.Resemble(expected))
	})

	ftt.Run("returns non-nil error for incorrectly formatted YAML", t, func(t *ftt.Test) {

		t.Run("missing top level `indexes` key", func(t *ftt.Test) {
			yaml := `
- kind: Cat
  properties:
  - name: name
  - name: age
    direction: desc
`
			_, err := ParseIndexYAML(bytes.NewBuffer([]byte(yaml)))
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("missing `name` key in property", func(t *ftt.Test) {
			yaml := `
indexes:

- kind: Cat
  ancestor: no
  properties:
  - name: name
  - direction: desc
`
			_, err := ParseIndexYAML(bytes.NewBuffer([]byte(yaml)))
			assert.Loosely(t, err, should.NotBeNil)
		})
	})
}

func TestFindAndParseIndexYAML(t *testing.T) {
	t.Parallel()

	ftt.Run("returns parsed index definitions for existing index YAML files", t, func(t *ftt.Test) {
		// YAML content to write temporarily to disk
		yaml1 := `
indexes:

- kind: Test Same Level
  properties:
  - name: name
  - name: age
    direction: desc
`
		yaml2 := `
indexes:

- kind: Test Higher Level
  properties:
  - name: name
  - name: age
    direction: desc

- kind: Test Foo
  properties:
  - name: height
  - name: weight
    direction: asc
`
		// determine the directory of this test file
		_, path, _, ok := runtime.Caller(0)
		if !ok {
			panic(fmt.Errorf("failed to determine test file path"))
		}
		sameLevelDir := filepath.Dir(path)

		t.Run("picks YAML file at same level as test file instead of higher level YAML file", func(t *ftt.Test) {
			writePath1 := filepath.Join(sameLevelDir, "index.yml")
			writePath2 := filepath.Join(filepath.Dir(sameLevelDir), "index.yaml")

			setup := func() {
				os.WriteFile(writePath1, []byte(yaml1), 0600)
				os.WriteFile(writePath2, []byte(yaml2), 0600)
			}

			cleanup := func() {
				os.Remove(writePath1)
				os.Remove(writePath2)
			}

			setup()
			defer cleanup()
			ids, err := FindAndParseIndexYAML(".")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids[0].Kind, should.Equal("Test Same Level"))
		})

		t.Run("finds YAML file two levels up given an empty relative path", func(t *ftt.Test) {
			writePath := filepath.Join(filepath.Dir(filepath.Dir(sameLevelDir)), "index.yaml")

			setup := func() {
				os.WriteFile(writePath, []byte(yaml2), 0600)
			}

			cleanup := func() {
				os.Remove(writePath)
			}

			setup()
			defer cleanup()
			ids, err := FindAndParseIndexYAML("")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids[1].Kind, should.Equal("Test Foo"))
		})

		t.Run("finds YAML file given a relative path", func(t *ftt.Test) {
			writeDir, err := ioutil.TempDir(filepath.Dir(sameLevelDir), "temp-test-datastore-")
			if err != nil {
				panic(err)
			}
			writePath := filepath.Join(writeDir, "index.yml")

			setup := func() {
				os.WriteFile(writePath, []byte(yaml2), 0600)
			}

			cleanup := func() {
				os.RemoveAll(writeDir)
			}

			setup()
			defer cleanup()
			ids, err := FindAndParseIndexYAML(filepath.Join("..", filepath.Base(writeDir)))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids[1].Kind, should.Equal("Test Foo"))
		})

		t.Run("finds YAML file given an absolute path", func(t *ftt.Test) {
			writePath := filepath.Join(sameLevelDir, "index.yaml")

			setup := func() {
				os.WriteFile(writePath, []byte(yaml2), 0600)
			}

			cleanup := func() {
				os.Remove(writePath)
			}

			setup()
			defer cleanup()

			abs, err := filepath.Abs(sameLevelDir)
			if err != nil {
				panic(fmt.Errorf("failed to find absolute path for `%s`", sameLevelDir))
			}

			ids, err := FindAndParseIndexYAML(abs)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids[1].Kind, should.Equal("Test Foo"))
		})
	})
}

func TestIteratorHeap(t *testing.T) {
	t.Parallel()

	ftt.Run("iteratorHeap", t, func(t *ftt.Test) {
		h := &iteratorHeap{}

		heap.Init(h)
		heap.Push(h, &queryIterator{currentItemOrderCache: "bb"})
		heap.Push(h, &queryIterator{currentItemOrderCache: "aa"})
		assert.Loosely(t, h.Len(), should.Equal(2))

		var res []*queryIterator
		for h.Len() > 0 {
			res = append(res, heap.Pop(h).(*queryIterator))
		}
		assert.Loosely(t, res, should.Resemble([]*queryIterator{
			{currentItemOrderCache: "aa"},
			{currentItemOrderCache: "bb"},
		}))
	})
}

// fakeDatastore2 extends the fakeDatastore but overrides the `raw.Run()` method for the `ds.RunMulti()` usage.
type fakeDatastore2 struct {
	fakeDatastore
}

func (f *fakeDatastore2) factory() RawFactory {
	return func(ic context.Context) RawInterface {
		fds := *f
		fds.kctx = GetKeyContext(ic)
		return &fds
	}
}

type Foo struct {
	_kind string `gae:"$kind,Foo"`
	ID    int64  `gae:"$id"`

	Values []string `gae:"values"`
	Val    int      `gae:"val"`
}

// prepare db data for fakeDatastore2
var (
	foo1 = &Foo{ID: 1, Values: []string{"aa"}, Val: 111}
	foo2 = &Foo{ID: 2, Values: []string{"bb", "cc"}, Val: 222}
	foo3 = &Foo{ID: 3, Values: []string{"cc"}, Val: 333}

	kc     = KeyContext{}
	keyMap = map[string][]*Key{
		"aa": {
			kc.MakeKey("Foo", foo1.ID),
		},
		"bb": {
			kc.MakeKey("Foo", foo2.ID),
		},
		"cc": {
			kc.MakeKey("Foo", foo2.ID),
			kc.MakeKey("Foo", foo3.ID),
		},
	}

	pmMap = map[string][]PropertyMap{
		"aa": {
			{"values": PropertySlice{MkProperty(foo1.Values[0])}, "val": MkProperty(foo1.Val)},
		},
		"bb": {
			{"values": PropertySlice{MkProperty(foo2.Values[0]), MkProperty(foo2.Values[1])}, "val": MkProperty(foo2.Val)},
		},
		"cc": {
			{"values": PropertySlice{MkProperty(foo2.Values[0]), MkProperty(foo2.Values[1])}, "val": MkProperty(foo2.Val)},
			{"values": PropertySlice{MkProperty(foo3.Values[0])}, "val": MkProperty(foo3.Val)},
		},
	}
)

func (f *fakeDatastore2) DecodeCursor(s string) (Cursor, error) {
	v, err := strconv.Atoi(s)
	return fakeCursor(v), err
}

func (f *fakeDatastore2) Run(fq *FinalizedQuery, cb RawRunCB) error {
	if _, ok := fq.eqFilts["@err_single"]; ok {
		return errors.New("errors in fakeDatastore")
	}

	if _, ok := fq.eqFilts["values"]; !ok {
		return nil
	}

	first := int32(0)
	start, end := fq.Bounds()
	if start != nil {
		first = int32(start.(fakeCursor))
	}

	searchStr := fq.eqFilts["values"][0].Value().(string)
	keys := make([]*Key, len(keyMap[searchStr]))
	pms := make([]PropertyMap, len(pmMap[searchStr]))
	copy(keys, keyMap[searchStr])
	copy(pms, pmMap[searchStr])

	last := int32(len(keys) - 1)
	if end != nil {
		last = int32(end.(fakeCursor))
	}

	if fq.orders[0].Property == "val" && fq.orders[0].Descending {
		// reverse
		for i, j := first, last; i < j; i, j = i+1, j-1 {
			keys[i], keys[j] = keys[j], keys[i]
			pms[i], pms[j] = pms[j], pms[i]
		}
	}

	for i := first; i <= last; i++ {
		// Cursor starts at the next available item.
		j := i + 1
		if err := cb(keys[i], pms[i], func() (Cursor, error) {
			return fakeCursor(j), nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func TestRunMulti(t *testing.T) {
	t.Parallel()
	ftt.Run("Test RunMulti", t, func(t *ftt.Test) {
		c := info.Set(context.Background(), fakeInfo{})
		fds2 := fakeDatastore2{}
		c = SetRawFactory(c, fds2.factory())

		t.Run("ok", func(t *ftt.Test) {
			t.Run("default - key ascending", func(t *ftt.Test) {
				queries := []*Query{
					NewQuery("Foo").Eq("values", "aa"),
					NewQuery("Foo").Eq("values", "cc"),
				}
				var foos []*Foo
				err := RunMulti(c, queries, func(foo *Foo) error {
					foos = append(foos, foo)
					return nil
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, foos, should.Resemble([]*Foo{foo1, foo2, foo3}))
			})

			t.Run("values field descending", func(t *ftt.Test) {
				queries := []*Query{
					NewQuery("Foo").Eq("values", "aa").Order("-val"),
					NewQuery("Foo").Eq("values", "cc").Order("-val"),
				}
				var foos []*Foo
				err := RunMulti(c, queries, func(foo *Foo) error {
					foos = append(foos, foo)
					return nil
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, foos, should.Resemble([]*Foo{foo3, foo2, foo1}))
			})

			t.Run("users send stop signal", func(t *ftt.Test) {
				queries := []*Query{
					NewQuery("Foo").Eq("values", "aa"),
					NewQuery("Foo").Eq("values", "cc"),
				}
				var foos []*Foo
				err := RunMulti(c, queries, func(foo *Foo) error {
					if len(foos) >= 2 {
						return Stop
					}
					foos = append(foos, foo)
					return nil
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, foos, should.Resemble([]*Foo{foo1, foo2}))
			})

			t.Run("not found", func(t *ftt.Test) {
				queries := []*Query{
					NewQuery("Foo").Eq("no_exist", "aa"),
					NewQuery("Foo").Eq("no_exist", "bb"),
				}
				var foos []*Foo
				err := RunMulti(c, queries, func(foo *Foo) error {
					foos = append(foos, foo)
					return nil
				})
				assert.Loosely(t, err, should.ErrLike(nil))
				assert.Loosely(t, foos, should.BeNil)
			})

			t.Run("default - key ascending, With Cursor", func(t *ftt.Test) {
				queries := []*Query{
					NewQuery("Foo").Eq("values", "aa"),
					NewQuery("Foo").Eq("values", "cc"),
				}
				var foos []*Foo
				err := RunMulti(c, queries, func(foo *Foo, cb CursorCB) error {
					foos = append(foos, foo)
					cur, err := cb()
					if err != nil {
						return err
					}
					if len(foos) == 1 {
						// Update the queries with the cursor
						queries, err = ApplyCursors(c, queries, cur)
						if err != nil {
							return err
						}
						// Stop after reading one entity
						return Stop
					}
					return nil
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, foos, should.Resemble([]*Foo{foo1}))
				// Get the remaining entity
				err = RunMulti(c, queries, func(foo *Foo, cb CursorCB) error {
					foos = append(foos, foo)
					return nil
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, foos, should.Resemble([]*Foo{foo1, foo2, foo3}))
			})

			t.Run("Query order - key ascending, With Cursor", func(t *ftt.Test) {
				queries := []*Query{
					NewQuery("Foo").Eq("values", "aa"),
					NewQuery("Foo").Eq("values", "cc"),
				}
				// Order is flipped for the next query
				queries1 := []*Query{
					NewQuery("Foo").Eq("values", "cc"),
					NewQuery("Foo").Eq("values", "aa"),
				}
				var foos []*Foo
				err := RunMulti(c, queries, func(foo *Foo, cb CursorCB) error {
					foos = append(foos, foo)
					cur, err := cb()
					if err != nil {
						return err
					}
					if len(foos) == 1 {
						// Update the queries1 with the cursor
						queries1, err = ApplyCursorString(c, queries1, cur.String())
						if err != nil {
							return err
						}
						// Stop after reading one entity
						return Stop
					}
					return nil
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, foos, should.Resemble([]*Foo{foo1}))
				// Get the remaining entities
				err = RunMulti(c, queries1, func(foo *Foo, cb CursorCB) error {
					foos = append(foos, foo)
					return nil
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, foos, should.Resemble([]*Foo{foo1, foo2, foo3}))
			})

			t.Run("Query cardinality - key ascending, With Cursor", func(t *ftt.Test) {
				queries := []*Query{
					NewQuery("Foo").Eq("values", "aa"),
					NewQuery("Foo").Eq("values", "cc"),
				}
				// Only one query in the second one
				queries1 := []*Query{
					NewQuery("Foo").Eq("values", "cc"),
				}
				var foos []*Foo
				var cur Cursor
				var err error
				err = RunMulti(c, queries, func(foo *Foo, cb CursorCB) error {
					foos = append(foos, foo)
					cur, err = cb()
					if err != nil {
						return err
					}
					if len(foos) == 2 {
						// Stop after reading one entity
						return Stop
					}
					return nil
				})
				assert.Loosely(t, foos, should.Resemble([]*Foo{foo1, foo2}))
				assert.Loosely(t, IsMultiCursor(cur), should.BeTrue)
				assert.Loosely(t, IsMultiCursorString(cur.String()), should.BeTrue)
				// Update the queries1 with the cursor
				_, err = ApplyCursors(c, queries1, cur)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err, should.ErrLike("Length mismatch. Cannot apply this cursor to the queries"))
			})

			t.Run("fake cursor fail", func(t *ftt.Test) {
				queries := []*Query{
					NewQuery("Foo").Eq("values", "aa"),
					NewQuery("Foo").Eq("values", "cc"),
				}
				// Apply cursor expects a multi cursor format
				cur := fakeCursor(100)
				// Update the queries1 with the cursor
				_, err := ApplyCursors(c, queries, cur)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err, should.ErrLike("Failed to decode cursor"))
				assert.Loosely(t, IsMultiCursor(cur), should.BeFalse)
				assert.Loosely(t, IsMultiCursorString(cur.String()), should.BeFalse)
			})

			t.Run("Query of different kinds", func(t *ftt.Test) {
				queries := []*Query{
					NewQuery("Foo"),
					NewQuery("Foo1"),
				}
				err := RunMulti(c, queries, func(foo *Foo, cb CursorCB) error {
					return nil
				})
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err, should.ErrLike("should query the same kind"))
			})
		})

		t.Run("errors in one running query", func(t *ftt.Test) {
			queries := []*Query{
				NewQuery("Foo").Eq("values", "aa"),
				NewQuery("Foo").Eq("@err_single", "error"),
			}
			var foos []*Foo
			err := RunMulti(c, queries, func(foo *Foo) error {
				foos = append(foos, foo)
				return nil
			})
			assert.Loosely(t, err, should.ErrLike("errors in fakeDatastore"))
		})
	})
}

func TestCountMulti(t *testing.T) {
	t.Parallel()

	ftt.Run("Test CountMulti", t, func(t *ftt.Test) {
		c := info.Set(context.Background(), fakeInfo{})
		fds2 := fakeDatastore2{}
		c = SetRawFactory(c, fds2.factory())
		count, err := CountMulti(c, []*Query{
			NewQuery("Foo").Eq("values", "aa"),
			NewQuery("Foo").Eq("values", "cc"),
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, count, should.Equal(3))
	})
}
