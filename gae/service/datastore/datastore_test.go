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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/common/errors"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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
	convey       C
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
		if v, ok := fq.eqFilts["$err_single"]; ok {
			idx := fq.eqFilts["$err_single_idx"][0].Value().(int64)
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
	so := So
	if f.convey != nil {
		so = f.convey.So
	}

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
				so(vals[i].Slice("Extra"), ShouldResemble, PropertySlice{MkProperty("whoa")})
			}

			if k.Kind() == "Index" {
				// Index types have a Value field. Generate a Key whose IntID equals
				// that Value field.
				k = k.KeyContext().NewKey(k.Kind(), "", vals[i]["Value"].Slice()[0].Value().(int64), k.Parent())
			} else {
				so(vals[i].Slice("Value"), ShouldResemble, PropertySlice{MkProperty(i)})
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
		So(ret.SetMeta("id", id), ShouldBeTrue)
		if f.Kind == "" {
			So(ret.SetMeta("kind", "FakePLS"), ShouldBeTrue)
		} else {
			So(ret.SetMeta("kind", f.Kind), ShouldBeTrue)
		}
		So(ret.SetMeta("assertExtra", true), ShouldBeTrue)
	}
	return ret, nil
}

func (f *FakePLS) GetMeta(key string) (interface{}, bool) {
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

func (f *FakePLS) SetMeta(key string, val interface{}) bool {
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

func (c plsChan) Load(pm PropertyMap) error                { return nil }
func (c plsChan) Save(withMeta bool) (PropertyMap, error)  { return nil, nil }
func (c plsChan) SetMeta(key string, val interface{}) bool { return false }
func (c plsChan) Problem() error                           { return nil }

func (c plsChan) GetMeta(key string) (interface{}, bool) {
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

func (s *MGSWithNoKind) GetMeta(key string) (interface{}, bool) {
	return nil, false
}

func (s *MGSWithNoKind) GetAllMeta() PropertyMap {
	return PropertyMap{"$kind": MkProperty("ohai")}
}

func (s *MGSWithNoKind) SetMeta(key string, val interface{}) bool {
	return false
}

var _ MetaGetterSetter = (*MGSWithNoKind)(nil)

func TestKeyForObj(t *testing.T) {
	t.Parallel()

	Convey("Test interface.KeyForObj", t, func() {
		c := info.Set(context.Background(), fakeInfo{})
		fds := fakeDatastore{}
		c = SetRawFactory(c, fds.factory())

		k := MakeKey(c, "Hello", "world")

		Convey("good", func() {
			Convey("struct containing $key", func() {
				type keyStruct struct {
					Key *Key `gae:"$key"`
				}

				ks := &keyStruct{k}
				So(KeyForObj(c, ks), ShouldEqual, k)
			})

			Convey("struct containing default $id and $kind", func() {
				type idStruct struct {
					id  string `gae:"$id,wut"`
					knd string `gae:"$kind,SuperKind"`
				}

				So(KeyForObj(c, &idStruct{}).String(), ShouldEqual, `s~aid:ns:/SuperKind,"wut"`)
			})

			Convey("struct containing $id and $parent", func() {
				So(KeyForObj(c, &CommonStruct{ID: 4}).String(), ShouldEqual, `s~aid:ns:/CommonStruct,4`)

				So(KeyForObj(c, &CommonStruct{ID: 4, Parent: k}).String(), ShouldEqual, `s~aid:ns:/Hello,"world"/CommonStruct,4`)
			})

			Convey("a propmap with $key", func() {
				pm := PropertyMap{}
				So(pm.SetMeta("key", k), ShouldBeTrue)
				So(KeyForObj(c, pm).String(), ShouldEqual, `s~aid:ns:/Hello,"world"`)
			})

			Convey("a propmap with $id, $kind, $parent", func() {
				pm := PropertyMap{}
				So(pm.SetMeta("id", 100), ShouldBeTrue)
				So(pm.SetMeta("kind", "Sup"), ShouldBeTrue)
				So(KeyForObj(c, pm).String(), ShouldEqual, `s~aid:ns:/Sup,100`)

				So(pm.SetMeta("parent", k), ShouldBeTrue)
				So(KeyForObj(c, pm).String(), ShouldEqual, `s~aid:ns:/Hello,"world"/Sup,100`)
			})

			Convey("a pls with $id, $parent", func() {
				pls := GetPLS(&CommonStruct{ID: 1})
				So(KeyForObj(c, pls).String(), ShouldEqual, `s~aid:ns:/CommonStruct,1`)

				So(pls.SetMeta("parent", k), ShouldBeTrue)
				So(KeyForObj(c, pls).String(), ShouldEqual, `s~aid:ns:/Hello,"world"/CommonStruct,1`)
			})
		})

		Convey("bad", func() {
			Convey("a propmap without $kind", func() {
				pm := PropertyMap{}
				So(pm.SetMeta("id", 100), ShouldBeTrue)
				So(func() { KeyForObj(c, pm) }, ShouldPanic)
			})

			Convey("a bad object", func() {
				type BadObj struct {
					ID int64 `gae:"$id"`

					NonSerializableField complex64
				}

				So(func() { KeyForObj(c, &BadObj{ID: 1}) }, ShouldPanicLike,
					`field "NonSerializableField" has invalid type: complex64`)
			})
		})
	})
}

func TestPopulateKey(t *testing.T) {
	t.Parallel()

	Convey("Test PopulateKey", t, func() {
		kc := MkKeyContext("app", "namespace")
		k := kc.NewKey("kind", "", 1337, nil)

		Convey("Can set the key of a common struct.", func() {
			var cs CommonStruct

			So(PopulateKey(&cs, k), ShouldBeTrue)
			So(cs.ID, ShouldEqual, 1337)
		})

		Convey("Can set the parent key of a const id struct.", func() {
			var s ConstIDStruct
			k2 := kc.NewKey("Bar", "bar", 0, k)
			PopulateKey(&s, k2)
			So(s.Parent, ShouldResemble, k)
		})

		Convey("Will not set the value of a singleton struct.", func() {
			var ss SingletonStruct

			So(PopulateKey(&ss, k), ShouldBeFalse)
			So(ss.id, ShouldEqual, 0)
		})

		Convey("Will panic when setting the key of a bad struct.", func() {
			var bs badStruct

			So(func() { PopulateKey(&bs, k) }, ShouldPanic)
		})

		Convey("Will panic when setting the key of a broken PLS struct.", func() {
			var broken permaBad

			So(func() { PopulateKey(&broken, k) }, ShouldPanic)
		})
	})
}

func TestAllocateIDs(t *testing.T) {
	t.Parallel()

	Convey("A testing environment", t, func() {
		c := info.Set(context.Background(), fakeInfo{})
		fds := fakeDatastore{}
		c = SetRawFactory(c, fds.factory())

		Convey("Testing AllocateIDs", func() {
			Convey("Will return nil if no entities are supplied.", func() {
				So(AllocateIDs(c), ShouldBeNil)
			})

			Convey("single struct", func() {
				cs := CommonStruct{Value: 1}
				So(AllocateIDs(c, &cs), ShouldBeNil)
				So(cs.ID, ShouldEqual, 1)
			})

			Convey("struct slice", func() {
				csSlice := []*CommonStruct{{Value: 1}, {Value: 2}}
				So(AllocateIDs(c, csSlice), ShouldBeNil)
				So(csSlice, ShouldResemble, []*CommonStruct{{ID: 1, Value: 1}, {ID: 2, Value: 2}})
			})

			Convey("single key will fail", func() {
				singleKey := MakeKey(c, "FooParent", "BarParent", "Foo", "Bar")
				So(func() { AllocateIDs(c, singleKey) }, ShouldPanicLike,
					"invalid input type (*datastore.Key): not a PLS, pointer-to-struct, or slice thereof")
			})

			Convey("key slice", func() {
				k0 := MakeKey(c, "Foo", "Bar")
				k1 := MakeKey(c, "Baz", "Qux")
				keySlice := []*Key{k0, k1}
				So(AllocateIDs(c, keySlice), ShouldBeNil)
				So(keySlice[0].Equal(MakeKey(c, "Foo", 1)), ShouldBeTrue)
				So(keySlice[1].Equal(MakeKey(c, "Baz", 2)), ShouldBeTrue)

				// The original keys should not have changed.
				So(k0.Equal(MakeKey(c, "Foo", "Bar")), ShouldBeTrue)
				So(k1.Equal(MakeKey(c, "Baz", "Qux")), ShouldBeTrue)
			})

			Convey("fail all key slice", func() {
				keySlice := []*Key{MakeKey(c, "FailAll", "oops"), MakeKey(c, "Baz", "Qux")}
				So(AllocateIDs(c, keySlice), ShouldEqual, errFailAll)
				So(keySlice[0].StringID(), ShouldEqual, "oops")
				So(keySlice[1].StringID(), ShouldEqual, "Qux")
			})

			Convey("fail key slice", func() {
				keySlice := []*Key{MakeKey(c, "Fail", "oops"), MakeKey(c, "Baz", "Qux")}
				So(AllocateIDs(c, keySlice), ShouldResemble, errors.MultiError{errFail, nil})
				So(keySlice[0].StringID(), ShouldEqual, "oops")
				So(keySlice[1].IntID(), ShouldEqual, 2)
			})

			Convey("vararg with errors", func() {
				successSlice := []CommonStruct{{Value: 0}, {Value: 1}}
				failSlice := []FakePLS{{Kind: "Fail"}, {Value: 3}}
				emptySlice := []CommonStruct(nil)
				cs0 := CommonStruct{Value: 4}
				cs1 := FakePLS{Kind: "Fail", Value: 5}
				keySlice := []*Key{MakeKey(c, "Foo", "Bar"), MakeKey(c, "Baz", "Qux")}
				fpls := FakePLS{StringID: "ohai", Value: 6}

				err := AllocateIDs(c, successSlice, failSlice, emptySlice, &cs0, &cs1, keySlice, &fpls)
				So(err, ShouldResemble, errors.MultiError{
					nil, errors.MultiError{errFail, nil}, nil, nil, errFail, nil, nil})
				So(successSlice[0].ID, ShouldEqual, 1)
				So(successSlice[1].ID, ShouldEqual, 2)
				So(failSlice[1].IntID, ShouldEqual, 4)
				So(cs0.ID, ShouldEqual, 5)
				So(keySlice[0].Equal(MakeKey(c, "Foo", 7)), ShouldBeTrue)
				So(keySlice[1].Equal(MakeKey(c, "Baz", 8)), ShouldBeTrue)
				So(fpls.IntID, ShouldEqual, 9)
			})
		})
	})
}

func TestPut(t *testing.T) {
	t.Parallel()

	Convey("A testing environment", t, func() {
		c := info.Set(context.Background(), fakeInfo{})
		fds := fakeDatastore{}
		c = SetRawFactory(c, fds.factory())

		Convey("Testing Put", func() {
			Convey("bad", func() {
				Convey("static can't serialize", func() {
					bss := []badStruct{{}, {}}
					So(func() { Put(c, bss) }, ShouldPanicLike,
						`field "Compy" has invalid type`)
				})

				Convey("static ptr can't serialize", func() {
					bss := []*badStruct{{}, {}}
					So(func() { Put(c, bss) }, ShouldPanicLike,
						`field "Compy" has invalid type: complex64`)
				})

				Convey("static bad type", func() {
					So(func() { Put(c, 100) }, ShouldPanicLike,
						"invalid input type (int): not a PLS, pointer-to-struct, or slice thereof")
				})

				Convey("static bad type (slice of bad type)", func() {
					So(func() { Put(c, []int{}) }, ShouldPanicLike,
						"invalid input type ([]int): not a PLS, pointer-to-struct, or slice thereof")
				})

				Convey("dynamic can't serialize", func() {
					fplss := []FakePLS{{failSave: true}, {}}
					So(Put(c, fplss), ShouldErrLike, "FakePLS.Save")
				})

				Convey("can't get keys", func() {
					fplss := []FakePLS{{failGetMeta: true}, {}}
					So(Put(c, fplss), ShouldErrLike, "unable to extract $kind")
				})

				Convey("get single error for RPC failure", func() {
					fplss := []FakePLS{{Kind: "FailAll"}, {}}
					So(Put(c, fplss), ShouldEqual, errFailAll)
				})

				Convey("get multi error for individual failures", func() {
					fplss := []FakePLS{{}, {Kind: "Fail"}}
					So(Put(c, fplss), ShouldResemble, errors.MultiError{nil, errFail})
				})

				Convey("get with *Key is an error", func() {
					So(func() { Get(c, &Key{}) }, ShouldPanicLike,
						"invalid input type (*datastore.Key): not a PLS, pointer-to-struct, or slice thereof")
				})

				Convey("struct with no $kind is an error", func() {
					s := MGSWithNoKind{}
					So(Put(c, &s), ShouldErrLike, "unable to extract $kind")
				})

				Convey("struct with invalid but non-nil key is an error", func() {
					type BadParent struct {
						ID     int64 `gae:"$id"`
						Parent *Key  `gae:"$parent"`
					}
					// having an Incomplete parent makes an invalid key
					bp := &BadParent{ID: 1, Parent: MakeKey(c, "Something", 0)}
					So(IsErrInvalidKey(Put(c, bp)), ShouldBeTrue)
				})

				Convey("vararg with errors", func() {
					successSlice := []CommonStruct{{Value: 0}, {Value: 1}}
					failSlice := []FakePLS{{Kind: "Fail"}, {Value: 3}}
					emptySlice := []CommonStruct(nil)
					cs0 := CommonStruct{Value: 4}
					failPLS := FakePLS{Kind: "Fail", Value: 5}
					fpls := FakePLS{StringID: "ohai", Value: 6}

					err := Put(c, successSlice, failSlice, emptySlice, &cs0, &failPLS, &fpls)
					So(err, ShouldResemble, errors.MultiError{
						nil, errors.MultiError{errFail, nil}, nil, nil, errFail, nil})
					So(successSlice[0].ID, ShouldEqual, 1)
					So(successSlice[1].ID, ShouldEqual, 2)
					So(cs0.ID, ShouldEqual, 5)
				})
			})

			Convey("ok", func() {
				Convey("[]S", func() {
					css := make([]CommonStruct, 7)
					for i := range css {
						if i == 4 {
							css[i].ID = 200
						}
						css[i].Value = int64(i)
					}
					So(Put(c, css), ShouldBeNil)
					for i, cs := range css {
						expect := int64(i + 1)
						if i == 4 {
							expect = 200
						}
						So(cs.ID, ShouldEqual, expect)
					}
				})

				Convey("[]*S", func() {
					css := make([]*CommonStruct, 7)
					for i := range css {
						css[i] = &CommonStruct{Value: int64(i)}
						if i == 4 {
							css[i].ID = 200
						}
					}
					So(Put(c, css), ShouldBeNil)
					for i, cs := range css {
						expect := int64(i + 1)
						if i == 4 {
							expect = 200
						}
						So(cs.ID, ShouldEqual, expect)
					}

					s := &CommonStruct{}
					So(Put(c, s), ShouldBeNil)
					So(s.ID, ShouldEqual, 1)
				})

				Convey("[]P", func() {
					fplss := make([]FakePLS, 7)
					for i := range fplss {
						fplss[i].Value = int64(i)
						if i == 4 {
							fplss[i].IntID = int64(200)
						}
					}
					So(Put(c, fplss), ShouldBeNil)
					for i, fpls := range fplss {
						expect := int64(i + 1)
						if i == 4 {
							expect = 200
						}
						So(fpls.IntID, ShouldEqual, expect)
					}

					pm := PropertyMap{"Value": MkProperty(0), "$kind": MkPropertyNI("Pmap")}
					So(Put(c, pm), ShouldBeNil)
					So(KeyForObj(c, pm).IntID(), ShouldEqual, 1)
				})

				Convey("[]P (map)", func() {
					pms := make([]PropertyMap, 7)
					for i := range pms {
						pms[i] = PropertyMap{
							"$kind": MkProperty("Pmap"),
							"Value": MkProperty(i),
						}
						if i == 4 {
							So(pms[i].SetMeta("id", int64(200)), ShouldBeTrue)
						}
					}
					So(Put(c, pms), ShouldBeNil)
					for i, pm := range pms {
						expect := int64(i + 1)
						if i == 4 {
							expect = 200
						}
						So(KeyForObj(c, pm).String(), ShouldEqual, fmt.Sprintf("s~aid:ns:/Pmap,%d", expect))
					}
				})

				Convey("[]*P", func() {
					fplss := make([]*FakePLS, 7)
					for i := range fplss {
						fplss[i] = &FakePLS{Value: int64(i)}
						if i == 4 {
							fplss[i].IntID = int64(200)
						}
					}
					So(Put(c, fplss), ShouldBeNil)
					for i, fpls := range fplss {
						expect := int64(i + 1)
						if i == 4 {
							expect = 200
						}
						So(fpls.IntID, ShouldEqual, expect)
					}
				})

				Convey("[]*P (map)", func() {
					pms := make([]*PropertyMap, 7)
					for i := range pms {
						pms[i] = &PropertyMap{
							"$kind": MkProperty("Pmap"),
							"Value": MkProperty(i),
						}
						if i == 4 {
							So(pms[i].SetMeta("id", int64(200)), ShouldBeTrue)
						}
					}
					So(Put(c, pms), ShouldBeNil)
					for i, pm := range pms {
						expect := int64(i + 1)
						if i == 4 {
							expect = 200
						}
						So(KeyForObj(c, *pm).String(), ShouldEqual, fmt.Sprintf("s~aid:ns:/Pmap,%d", expect))
					}
				})

				Convey("[]I", func() {
					ifs := []interface{}{
						&CommonStruct{Value: 0},
						&FakePLS{Value: 1},
						PropertyMap{"Value": MkProperty(2), "$kind": MkPropertyNI("Pmap")},
						&PropertyMap{"Value": MkProperty(3), "$kind": MkPropertyNI("Pmap")},
					}
					So(Put(c, ifs), ShouldBeNil)
					for i := range ifs {
						switch i {
						case 0:
							So(ifs[i].(*CommonStruct).ID, ShouldEqual, 1)
						case 1:
							fpls := ifs[i].(*FakePLS)
							So(fpls.IntID, ShouldEqual, 2)
						case 2:
							So(KeyForObj(c, ifs[i].(PropertyMap)).String(), ShouldEqual, "s~aid:ns:/Pmap,3")
						case 3:
							So(KeyForObj(c, *ifs[i].(*PropertyMap)).String(), ShouldEqual, "s~aid:ns:/Pmap,4")
						}
					}
				})

				Convey("vararg (flat)", func() {
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
					So(err, ShouldResemble, nil)

					So(sa.ID, ShouldEqual, 1)
					So(sb._id, ShouldEqual, 1)
					So(sc._id, ShouldEqual, 0) // Could not be set, private.
				})
			})
		})
	})
}

func TestExists(t *testing.T) {
	t.Parallel()

	Convey("A testing environment", t, func() {
		c := info.Set(context.Background(), fakeInfo{})
		fds := fakeDatastore{}
		c = SetRawFactory(c, fds.factory())

		k := MakeKey(c, "Hello", "world")

		Convey("Exists", func() {
			// Single key.
			er, err := Exists(c, k)
			So(err, ShouldBeNil)
			So(er.All(), ShouldBeTrue)

			// Single key failure.
			_, err = Exists(c, MakeKey(c, "Fail", "boom"))
			So(err, ShouldEqual, errFail)

			// Single slice of keys.
			er, err = Exists(c, []*Key{k, MakeKey(c, "hello", "other")})
			So(err, ShouldBeNil)
			So(er.All(), ShouldBeTrue)

			// Single slice of keys failure.
			er, err = Exists(c, []*Key{k, MakeKey(c, "Fail", "boom")})
			So(err, ShouldResemble, errors.MultiError{nil, errFail})
			So(er.Get(0, 0), ShouldBeTrue)

			// Single key missing.
			er, err = Exists(c, MakeKey(c, "DNE", "nope"))
			So(err, ShouldBeNil)
			So(er.Any(), ShouldBeFalse)

			// Multi-arg keys with one missing.
			er, err = Exists(c, k, MakeKey(c, "DNE", "other"))
			So(err, ShouldBeNil)
			So(er.Get(0), ShouldBeTrue)
			So(er.Get(1), ShouldBeFalse)

			// Multi-arg keys with two missing.
			er, err = Exists(c, MakeKey(c, "DNE", "nope"), MakeKey(c, "DNE", "other"))
			So(err, ShouldBeNil)
			So(er.Any(), ShouldBeFalse)

			// Single struct pointer.
			er, err = Exists(c, &CommonStruct{ID: 1})
			So(err, ShouldBeNil)
			So(er.All(), ShouldBeTrue)

			// Multi-arg mixed key/struct/slices.
			er, err = Exists(c,
				&CommonStruct{ID: 1},
				[]*CommonStruct(nil),
				[]*Key{MakeKey(c, "DNE", "nope"), MakeKey(c, "hello", "ohai")},
			)
			So(err, ShouldBeNil)
			So(er.Get(0), ShouldBeTrue)
			So(er.Get(1), ShouldBeTrue)
			So(er.Get(2), ShouldBeFalse)
			So(er.Get(2, 0), ShouldBeFalse)
			So(er.Get(2, 1), ShouldBeTrue)
		})
	})
}

func TestDelete(t *testing.T) {
	t.Parallel()

	Convey("A testing environment", t, func() {
		c := info.Set(context.Background(), fakeInfo{})
		fds := fakeDatastore{}
		c = SetRawFactory(c, fds.factory())

		Convey("Testing Delete", func() {
			Convey("bad", func() {
				Convey("get single error for RPC failure", func() {
					keys := []*Key{
						MakeKey(c, "s~aid", "ns", "FailAll", 1),
						MakeKey(c, "s~aid", "ns", "Ok", 1),
					}
					So(Delete(c, keys), ShouldEqual, errFailAll)
				})

				Convey("get multi error for individual failure", func() {
					keys := []*Key{
						MakeKey(c, "Ok", 1),
						MakeKey(c, "Fail", 2),
					}
					So(Delete(c, keys), ShouldResemble, errors.MultiError{nil, errFail})
				})

				Convey("put with non-modifyable type is an error", func() {
					cs := CommonStruct{}
					So(func() { Put(c, cs) }, ShouldPanicLike,
						"invalid input type (datastore.CommonStruct): not a pointer")
				})

				Convey("get single error when deleting a single", func() {
					k := MakeKey(c, "Fail", 1)
					So(Delete(c, k), ShouldEqual, errFail)
				})
			})

			Convey("good", func() {
				// Single struct pointer.
				So(Delete(c, &CommonStruct{ID: 1}), ShouldBeNil)

				// Single key.
				So(Delete(c, MakeKey(c, "hello", "ohai")), ShouldBeNil)

				// Single struct DNE.
				So(Delete(c, &CommonStruct{ID: noSuchEntityID}), ShouldEqual, ErrNoSuchEntity)

				// Single key DNE.
				So(Delete(c, MakeKey(c, "DNE", "nope")), ShouldEqual, ErrNoSuchEntity)

				// Mixed key/struct/slices.
				err := Delete(c,
					&CommonStruct{ID: 1},
					[]*Key{MakeKey(c, "hello", "ohai"), MakeKey(c, "DNE", "nope")},
				)
				So(err, ShouldResemble, errors.MultiError{nil, errors.MultiError{nil, ErrNoSuchEntity}})
			})
		})
	})
}

func TestGet(t *testing.T) {
	t.Parallel()

	Convey("A testing environment", t, func() {
		c := info.Set(context.Background(), fakeInfo{})
		fds := fakeDatastore{}
		c = SetRawFactory(c, fds.factory())

		Convey("Testing Get", func() {
			Convey("bad", func() {
				Convey("static can't serialize", func() {
					toGet := []badStruct{{}, {}}
					So(func() { Get(c, toGet) }, ShouldPanicLike,
						`field "Compy" has invalid type: complex64`)
				})

				Convey("can't get keys", func() {
					fplss := []FakePLS{{failGetMeta: true}, {}}
					So(Get(c, fplss), ShouldErrLike, "unable to extract $kind")
				})

				Convey("get single error for RPC failure", func() {
					fplss := []FakePLS{
						{IntID: 1, Kind: "FailAll"},
						{IntID: 2},
					}
					So(Get(c, fplss), ShouldEqual, errFailAll)
				})

				Convey("get multi error for individual failures", func() {
					fplss := []FakePLS{{IntID: 1}, {IntID: 2, Kind: "Fail"}}
					So(Get(c, fplss), ShouldResemble, errors.MultiError{nil, errFail})
				})

				Convey("get with non-modifiable type is an error", func() {
					cs := CommonStruct{}
					So(func() { Get(c, cs) }, ShouldPanicLike,
						"invalid input type (datastore.CommonStruct): not a pointer")
				})

				Convey("get with nil is an error", func() {
					So(func() { Get(c, nil) }, ShouldPanicLike,
						"cannot use nil as single argument")
				})

				Convey("get with ptr-to-nonstruct is an error", func() {
					val := 100
					So(func() { Get(c, &val) }, ShouldPanicLike,
						"invalid input type (*int): not a PLS, pointer-to-struct, or slice thereof")
				})

				Convey("failure to save metadata is no problem though", func() {
					// It just won't save the key
					cs := &FakePLS{IntID: 10, failSetMeta: true}
					So(Get(c, cs), ShouldBeNil)
				})

				Convey("vararg with errors", func() {
					successSlice := []CommonStruct{{ID: 1}, {ID: 2}}
					failSlice := []CommonStruct{{ID: noSuchEntityID}, {ID: 3}}
					emptySlice := []CommonStruct(nil)
					cs0 := CommonStruct{ID: 4}
					failPLS := CommonStruct{ID: noSuchEntityID}
					fpls := FakePLS{StringID: "ohai"}

					err := Get(c, successSlice, failSlice, emptySlice, &cs0, &failPLS, &fpls)
					So(err, ShouldResemble, errors.MultiError{
						nil, errors.MultiError{ErrNoSuchEntity, nil}, nil, nil, ErrNoSuchEntity, nil})
					So(successSlice[0].Value, ShouldEqual, 1)
					So(successSlice[1].Value, ShouldEqual, 2)
					So(cs0.Value, ShouldEqual, 5)
					So(fpls.Value, ShouldEqual, 7)
				})
			})

			Convey("ok", func() {
				Convey("Get", func() {
					cs := &CommonStruct{ID: 1}
					So(Get(c, cs), ShouldBeNil)
					So(cs.Value, ShouldEqual, 1)
				})

				Convey("Raw access too", func() {
					rds := Raw(c)
					keys := []*Key{MakeKey(c, "Kind", 1)}
					So(rds.GetMulti(keys, nil, func(_ int, pm PropertyMap, err error) error {
						So(err, ShouldBeNil)
						So(pm.Slice("Value")[0].Value(), ShouldEqual, 1)
						return nil
					}), ShouldBeNil)
				})

				Convey("but general failure to save is fine on a Get", func() {
					cs := &FakePLS{failSave: true, IntID: 7}
					So(Get(c, cs), ShouldBeNil)
				})

				Convey("vararg", func() {
					successSlice := []CommonStruct{{ID: 1}, {ID: 2}}
					cs := CommonStruct{ID: 3}

					err := Get(c, successSlice, &cs)
					So(err, ShouldBeNil)
					So(successSlice[0].Value, ShouldEqual, 1)
					So(successSlice[1].Value, ShouldEqual, 2)
					So(cs.Value, ShouldEqual, 3)
				})
			})
		})
	})
}

func TestGetAll(t *testing.T) {
	t.Parallel()

	Convey("Test GetAll", t, func() {
		c := info.Set(context.Background(), fakeInfo{})
		fds := fakeDatastore{}
		c = SetRawFactory(c, fds.factory())

		fds.entities = 5
		q := NewQuery("")

		Convey("bad", func() {
			Convey("nil target", func() {
				So(func() { GetAll(c, q, (*[]PropertyMap)(nil)) }, ShouldPanicLike,
					"invalid GetAll dst: <nil>")
			})

			Convey("bad type", func() {
				output := 100
				So(func() { GetAll(c, q, &output) }, ShouldPanicLike,
					"invalid argument type: expected slice, got int")
			})

			Convey("bad type (non pointer)", func() {
				So(func() { GetAll(c, q, "moo") }, ShouldPanicLike,
					"invalid GetAll dst: must have a ptr-to-slice")
			})

			Convey("bad type (underspecified)", func() {
				output := []PropertyLoadSaver(nil)
				So(func() { GetAll(c, q, &output) }, ShouldPanicLike,
					"invalid GetAll dst (non-concrete element type): *[]datastore.PropertyLoadSaver")
			})
		})

		Convey("ok", func() {
			Convey("*[]S", func() {
				output := []CommonStruct(nil)
				So(GetAll(c, q, &output), ShouldBeNil)
				So(len(output), ShouldEqual, 5)
				for i, o := range output {
					So(o.ID, ShouldEqual, i+1)
					So(o.Value, ShouldEqual, i)
				}
			})

			Convey("*[]*S", func() {
				output := []*CommonStruct(nil)
				So(GetAll(c, q, &output), ShouldBeNil)
				So(len(output), ShouldEqual, 5)
				for i, o := range output {
					So(o.ID, ShouldEqual, i+1)
					So(o.Value, ShouldEqual, i)
				}
			})

			Convey("*[]P", func() {
				output := []FakePLS(nil)
				So(GetAll(c, q, &output), ShouldBeNil)
				So(len(output), ShouldEqual, 5)
				for i, o := range output {
					So(o.gotLoaded, ShouldBeTrue)
					So(o.IntID, ShouldEqual, i+1)
					So(o.Value, ShouldEqual, i)
				}
			})

			Convey("*[]P (map)", func() {
				output := []PropertyMap(nil)
				So(GetAll(c, q, &output), ShouldBeNil)
				So(len(output), ShouldEqual, 5)
				for i, o := range output {
					k, ok := o.GetMeta("key")
					So(ok, ShouldBeTrue)
					So(k.(*Key).IntID(), ShouldEqual, i+1)
					So(o.Slice("Value")[0].Value().(int64), ShouldEqual, i)
				}
			})

			Convey("*[]P (chan)", func() {
				output := []plsChan(nil)
				So(GetAll(c, q, &output), ShouldBeNil)
				So(output, ShouldHaveLength, 5)
				for _, o := range output {
					So(KeyForObj(c, o).StringID(), ShouldEqual, "whyDoIExist")
				}
			})

			Convey("*[]*P", func() {
				output := []*FakePLS(nil)
				So(GetAll(c, q, &output), ShouldBeNil)
				So(len(output), ShouldEqual, 5)
				for i, o := range output {
					So(o.gotLoaded, ShouldBeTrue)
					So(o.IntID, ShouldEqual, i+1)
					So(o.Value, ShouldEqual, i)
				}
			})

			Convey("*[]*P (map)", func() {
				output := []*PropertyMap(nil)
				So(GetAll(c, q, &output), ShouldBeNil)
				So(len(output), ShouldEqual, 5)
				for i, op := range output {
					o := *op
					k, ok := o.GetMeta("key")
					So(ok, ShouldBeTrue)
					So(k.(*Key).IntID(), ShouldEqual, i+1)
					So(o.Slice("Value")[0].Value().(int64), ShouldEqual, i)
				}
			})

			Convey("*[]*P (chan)", func() {
				output := []*plsChan(nil)
				So(GetAll(c, q, &output), ShouldBeNil)
				So(output, ShouldHaveLength, 5)
				for _, o := range output {
					So(KeyForObj(c, o).StringID(), ShouldEqual, "whyDoIExist")
				}
			})

			Convey("*[]*Key", func() {
				output := []*Key(nil)
				So(GetAll(c, q, &output), ShouldBeNil)
				So(len(output), ShouldEqual, 5)
				for i, k := range output {
					So(k.IntID(), ShouldEqual, i+1)
				}
			})

		})
	})
}

func TestRun(t *testing.T) {
	t.Parallel()

	Convey("Test Run", t, func() {
		c := info.Set(context.Background(), fakeInfo{})
		fds := fakeDatastore{}
		c = SetRawFactory(c, fds.factory())

		fds.entities = 5
		q := NewQuery("kind")

		Convey("bad", func() {
			assertBadTypePanics := func(cb interface{}) {
				So(func() { Run(c, q, cb) }, ShouldPanicLike,
					"cb does not match the required callback signature")
			}

			Convey("not a function", func() {
				assertBadTypePanics("I am a potato")
			})

			Convey("nil", func() {
				assertBadTypePanics(nil)
			})

			Convey("interface", func() {
				assertBadTypePanics(func(pls PropertyLoadSaver) {})
			})

			Convey("bad proto type", func() {
				cb := func(v int) {
					panic("never here!")
				}
				So(func() { Run(c, q, cb) }, ShouldPanicLike,
					"invalid argument type: int is not a PLS or pointer-to-struct")
			})

			Convey("wrong # args", func() {
				assertBadTypePanics(func(v CommonStruct, _ CursorCB, _ int) {
					panic("never here!")
				})
			})

			Convey("wrong ret type", func() {
				assertBadTypePanics(func(v CommonStruct) bool {
					panic("never here!")
				})
			})

			Convey("wrong # rets", func() {
				assertBadTypePanics(func(v CommonStruct) (int, error) {
					panic("never here!")
				})
			})

			Convey("bad 2nd arg", func() {
				assertBadTypePanics(func(v CommonStruct, _ Cursor) error {
					panic("never here!")
				})
			})

			Convey("early abort on error", func() {
				q = q.Eq("$err_single", "Query fail").Eq("$err_single_idx", 3)
				i := 0
				So(Run(c, q, func(c CommonStruct) {
					i++
				}), ShouldErrLike, "Query fail")
				So(i, ShouldEqual, 3)
			})

			Convey("return error on serialization failure", func() {
				So(Run(c, q, func(_ permaBad) {
					panic("never here")
				}).Error(), ShouldEqual, "permaBad")
			})
		})

		Convey("ok", func() {
			Convey("can return error to stop", func() {
				i := 0
				So(Run(c, q, func(c CommonStruct) error {
					i++
					return Stop
				}), ShouldBeNil)
				So(i, ShouldEqual, 1)

				i = 0
				So(Run(c, q, func(c CommonStruct, _ CursorCB) error {
					i++
					return fmt.Errorf("my error")
				}), ShouldErrLike, "my error")
				So(i, ShouldEqual, 1)
			})

			Convey("Can optionally get cursor function", func() {
				i := 0
				So(Run(c, q, func(c CommonStruct, ccb CursorCB) {
					i++
					curs, err := ccb()
					So(err, ShouldBeNil)
					So(curs.String(), ShouldEqual, fakeCursor(i).String())
				}), ShouldBeNil)
				So(i, ShouldEqual, 5)
			})

			Convey("*S", func() {
				i := 0
				So(Run(c, q, func(cs *CommonStruct) {
					So(cs.ID, ShouldEqual, i+1)
					So(cs.Value, ShouldEqual, i)
					i++
				}), ShouldBeNil)
			})

			Convey("*P", func() {
				fds.keyForResult = func(i int32, kctx KeyContext) *Key {
					if i == 10 {
						return kctx.MakeKey("Kind", "eleven")
					}
					return kctx.MakeKey("Kind", i+1)
				}

				i := 0
				So(Run(c, q.Limit(12), func(fpls *FakePLS) {
					So(fpls.gotLoaded, ShouldBeTrue)
					if i == 10 {
						So(fpls.StringID, ShouldEqual, "eleven")
					} else {
						So(fpls.IntID, ShouldEqual, i+1)
					}
					So(fpls.Value, ShouldEqual, i)
					i++
				}), ShouldBeNil)
			})

			Convey("*P (map)", func() {
				i := 0
				So(Run(c, q, func(pm *PropertyMap) {
					k, ok := pm.GetMeta("key")
					So(ok, ShouldBeTrue)
					So(k.(*Key).IntID(), ShouldEqual, i+1)
					So((*pm).Slice("Value")[0].Value(), ShouldEqual, i)
					i++
				}), ShouldBeNil)
			})

			Convey("*P (chan)", func() {
				So(Run(c, q, func(ch *plsChan) {
					So(KeyForObj(c, ch).StringID(), ShouldEqual, "whyDoIExist")
				}), ShouldBeNil)
			})

			Convey("S", func() {
				i := 0
				So(Run(c, q, func(cs CommonStruct) {
					So(cs.ID, ShouldEqual, i+1)
					So(cs.Value, ShouldEqual, i)
					i++
				}), ShouldBeNil)
			})

			Convey("P", func() {
				i := 0
				So(Run(c, q, func(fpls FakePLS) {
					So(fpls.gotLoaded, ShouldBeTrue)
					So(fpls.IntID, ShouldEqual, i+1)
					So(fpls.Value, ShouldEqual, i)
					i++
				}), ShouldBeNil)
			})

			Convey("P (map)", func() {
				i := 0
				So(Run(c, q, func(pm PropertyMap) {
					k, ok := pm.GetMeta("key")
					So(ok, ShouldBeTrue)
					So(k.(*Key).IntID(), ShouldEqual, i+1)
					So(pm.Slice("Value")[0].Value(), ShouldEqual, i)
					i++
				}), ShouldBeNil)
			})

			Convey("P (chan)", func() {
				So(Run(c, q, func(ch plsChan) {
					So(KeyForObj(c, ch).StringID(), ShouldEqual, "whyDoIExist")
				}), ShouldBeNil)
			})

			Convey("Key", func() {
				i := 0
				So(Run(c, q, func(k *Key) {
					So(k.IntID(), ShouldEqual, i+1)
					i++
				}), ShouldBeNil)
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

	Convey("Test changing schemas", t, func() {
		fds := fixedDataDatastore{}
		c := info.Set(context.Background(), fakeInfo{})
		c = SetRaw(c, &fds)

		Convey("Can add fields", func() {
			initial := PropertyMap{
				"$key": mpNI(MakeKey(c, "Val", 10)),
				"Val":  mp(100),
			}
			So(Put(c, initial), ShouldBeNil)

			type Val struct {
				ID int64 `gae:"$id"`

				Val    int64
				TwoVal int64 // whoa, TWO vals! amazing
			}
			tv := &Val{ID: 10, TwoVal: 2}
			So(Get(c, tv), ShouldBeNil)
			So(tv, ShouldResemble, &Val{ID: 10, Val: 100, TwoVal: 2})
		})

		Convey("Removing fields", func() {
			initial := PropertyMap{
				"$key":   mpNI(MakeKey(c, "Val", 10)),
				"Val":    mp(100),
				"TwoVal": mp(200),
			}
			So(Put(c, initial), ShouldBeNil)

			Convey("is normally an error", func() {
				type Val struct {
					ID int64 `gae:"$id"`

					Val int64
				}
				tv := &Val{ID: 10}
				So(Get(c, tv), ShouldErrLike,
					`gae: cannot load field "TwoVal" into a "datastore.Val`)
				So(tv, ShouldResemble, &Val{ID: 10, Val: 100})
			})

			Convey("Unless you have an ,extra field!", func() {
				type Val struct {
					ID int64 `gae:"$id"`

					Val   int64
					Extra PropertyMap `gae:",extra"`
				}
				tv := &Val{ID: 10}
				So(Get(c, tv), ShouldBeNil)
				So(tv, ShouldResemble, &Val{
					ID:  10,
					Val: 100,
					Extra: PropertyMap{
						"TwoVal": PropertySlice{mp(200)},
					},
				})
			})
		})

		Convey("Can round-trip extra fields", func() {
			type Expando struct {
				ID int64 `gae:"$id"`

				Something int
				Extra     PropertyMap `gae:",extra"`
			}
			ex := &Expando{10, 17, PropertyMap{
				"Hello": mp("Hello"),
				"World": mp(true),
			}}
			So(Put(c, ex), ShouldBeNil)

			ex = &Expando{ID: 10}
			So(Get(c, ex), ShouldBeNil)
			So(ex, ShouldResemble, &Expando{
				ID:        10,
				Something: 17,
				Extra: PropertyMap{
					"Hello": PropertySlice{mp("Hello")},
					"World": PropertySlice{mp(true)},
				},
			})
		})

		Convey("Can read-but-not-write", func() {
			initial := PropertyMap{
				"$key":   mpNI(MakeKey(c, "Convert", 10)),
				"Val":    mp(100),
				"TwoVal": mp(200),
			}
			So(Put(c, initial), ShouldBeNil)
			type Convert struct {
				ID int64 `gae:"$id"`

				Val    int64
				NewVal int64
				Extra  PropertyMap `gae:"-,extra"`
			}
			cnv := &Convert{ID: 10}
			So(Get(c, cnv), ShouldBeNil)
			So(cnv, ShouldResemble, &Convert{
				ID: 10, Val: 100, NewVal: 0, Extra: PropertyMap{"TwoVal": PropertySlice{mp(200)}},
			})
			cnv.NewVal = cnv.Extra.Slice("TwoVal")[0].Value().(int64)
			So(Put(c, cnv), ShouldBeNil)

			cnv = &Convert{ID: 10}
			So(Get(c, cnv), ShouldBeNil)
			So(cnv, ShouldResemble, &Convert{
				ID: 10, Val: 100, NewVal: 200, Extra: nil,
			})
		})

		Convey("Can black hole", func() {
			initial := PropertyMap{
				"$key":   mpNI(MakeKey(c, "BlackHole", 10)),
				"Val":    mp(100),
				"TwoVal": mp(200),
			}
			So(Put(c, initial), ShouldBeNil)
			type BlackHole struct {
				ID int64 `gae:"$id"`

				NewStuff  string
				blackHole PropertyMap `gae:"-,extra"`
			}
			b := &BlackHole{ID: 10, NewStuff: "(╯°□°)╯︵ ┻━┻"}
			So(Get(c, b), ShouldBeNil)
			So(b, ShouldResemble, &BlackHole{ID: 10, NewStuff: "(╯°□°)╯︵ ┻━┻"})
		})

		Convey("Can change field types", func() {
			initial := PropertyMap{
				"$key": mpNI(MakeKey(c, "IntChange", 10)),
				"Val":  mp(100),
			}
			So(Put(c, initial), ShouldBeNil)

			type IntChange struct {
				ID    int64 `gae:"$id"`
				Val   string
				Extra PropertyMap `gae:"-,extra"`
			}
			i := &IntChange{ID: 10}
			So(Get(c, i), ShouldBeNil)
			So(i, ShouldResemble, &IntChange{ID: 10, Extra: PropertyMap{"Val": PropertySlice{mp(100)}}})
			i.Val = fmt.Sprint(i.Extra.Slice("Val")[0].Value())
			So(Put(c, i), ShouldBeNil)

			i = &IntChange{ID: 10}
			So(Get(c, i), ShouldBeNil)
			So(i, ShouldResemble, &IntChange{ID: 10, Val: "100"})
		})

		Convey("Native fields have priority over Extra fields", func() {
			type Dup struct {
				ID    int64 `gae:"$id"`
				Val   int64
				Extra PropertyMap `gae:",extra"`
			}
			d := &Dup{ID: 10, Val: 100, Extra: PropertyMap{
				"Val":   PropertySlice{mp(200)},
				"Other": PropertySlice{mp("other")},
			}}
			So(Put(c, d), ShouldBeNil)

			d = &Dup{ID: 10}
			So(Get(c, d), ShouldBeNil)
			So(d, ShouldResemble, &Dup{
				ID: 10, Val: 100, Extra: PropertyMap{"Other": PropertySlice{mp("other")}},
			})
		})

		Convey("Can change repeated field to non-repeating field", func() {
			initial := PropertyMap{
				"$key": mpNI(MakeKey(c, "NonRepeating", 10)),
				"Val":  PropertySlice{mp(100), mp(200), mp(400)},
			}
			So(Put(c, initial), ShouldBeNil)

			type NonRepeating struct {
				ID    int64 `gae:"$id"`
				Val   int64
				Extra PropertyMap `gae:",extra"`
			}
			n := &NonRepeating{ID: 10}
			So(Get(c, n), ShouldBeNil)
			So(n, ShouldResemble, &NonRepeating{
				ID: 10, Val: 0, Extra: PropertyMap{
					"Val": PropertySlice{mp(100), mp(200), mp(400)},
				},
			})
		})

		Convey("Deals correctly with recursive types", func() {
			initial := PropertyMap{
				"$key": mpNI(MakeKey(c, "Outer", 10)),
				"I.A":  PropertySlice{mp(1), mp(2), mp(4)},
				"I.B":  PropertySlice{mp(10), mp(20), mp(40)},
				"I.C":  PropertySlice{mp(100), mp(200), mp(400)},
			}
			So(Put(c, initial), ShouldBeNil)
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
			So(Get(c, o), ShouldBeNil)
			So(o, ShouldResemble, &Outer{
				ID: 10,
				I: []Inner{
					{1, 10},
					{2, 20},
					{4, 40},
				},
				Extra: PropertyMap{
					"I.C": PropertySlice{mp(100), mp(200), mp(400)},
				},
			})
		})

		Convey("Problems", func() {
			Convey("multiple extra fields", func() {
				type Bad struct {
					A PropertyMap `gae:",extra"`
					B PropertyMap `gae:",extra"`
				}
				So(func() { GetPLS(&Bad{}) }, ShouldPanicLike,
					"multiple fields tagged as 'extra'")
			})

			Convey("extra field with name", func() {
				type Bad struct {
					A PropertyMap `gae:"wut,extra"`
				}
				So(func() { GetPLS(&Bad{}) }, ShouldPanicLike,
					"struct 'extra' field has invalid name wut")
			})

			Convey("extra field with bad type", func() {
				type Bad struct {
					A int64 `gae:",extra"`
				}
				So(func() { GetPLS(&Bad{}) }, ShouldPanicLike,
					"struct 'extra' field has invalid type int64")
			})
		})
	})
}

func TestParseIndexYAML(t *testing.T) {
	t.Parallel()

	Convey("parses properly formatted YAML", t, func() {
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
		So(err, ShouldBeNil)

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
		So(ids, ShouldResemble, expected)
	})

	Convey("returns non-nil error for incorrectly formatted YAML", t, func() {

		Convey("missing top level `indexes` key", func() {
			yaml := `
- kind: Cat
  properties:
  - name: name
  - name: age
    direction: desc
`
			_, err := ParseIndexYAML(bytes.NewBuffer([]byte(yaml)))
			So(err, ShouldNotBeNil)
		})

		Convey("missing `name` key in property", func() {
			yaml := `
indexes:

- kind: Cat
  ancestor: no
  properties:
  - name: name
  - direction: desc
`
			_, err := ParseIndexYAML(bytes.NewBuffer([]byte(yaml)))
			So(err, ShouldNotBeNil)
		})
	})
}

func TestFindAndParseIndexYAML(t *testing.T) {
	t.Parallel()

	Convey("returns parsed index definitions for existing index YAML files", t, func() {
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

		Convey("picks YAML file at same level as test file instead of higher level YAML file", func() {
			writePath1 := filepath.Join(sameLevelDir, "index.yml")
			writePath2 := filepath.Join(filepath.Dir(sameLevelDir), "index.yaml")

			setup := func() {
				ioutil.WriteFile(writePath1, []byte(yaml1), 0600)
				ioutil.WriteFile(writePath2, []byte(yaml2), 0600)
			}

			cleanup := func() {
				os.Remove(writePath1)
				os.Remove(writePath2)
			}

			setup()
			defer cleanup()
			ids, err := FindAndParseIndexYAML(".")
			So(err, ShouldBeNil)
			So(ids[0].Kind, ShouldEqual, "Test Same Level")
		})

		Convey("finds YAML file two levels up given an empty relative path", func() {
			writePath := filepath.Join(filepath.Dir(filepath.Dir(sameLevelDir)), "index.yaml")

			setup := func() {
				ioutil.WriteFile(writePath, []byte(yaml2), 0600)
			}

			cleanup := func() {
				os.Remove(writePath)
			}

			setup()
			defer cleanup()
			ids, err := FindAndParseIndexYAML("")
			So(err, ShouldBeNil)
			So(ids[1].Kind, ShouldEqual, "Test Foo")
		})

		Convey("finds YAML file given a relative path", func() {
			writeDir, err := ioutil.TempDir(filepath.Dir(sameLevelDir), "temp-test-datastore-")
			if err != nil {
				panic(err)
			}
			writePath := filepath.Join(writeDir, "index.yml")

			setup := func() {
				ioutil.WriteFile(writePath, []byte(yaml2), 0600)
			}

			cleanup := func() {
				os.RemoveAll(writeDir)
			}

			setup()
			defer cleanup()
			ids, err := FindAndParseIndexYAML(filepath.Join("..", filepath.Base(writeDir)))
			So(err, ShouldBeNil)
			So(ids[1].Kind, ShouldEqual, "Test Foo")
		})

		Convey("finds YAML file given an absolute path", func() {
			writePath := filepath.Join(sameLevelDir, "index.yaml")

			setup := func() {
				ioutil.WriteFile(writePath, []byte(yaml2), 0600)
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
			So(err, ShouldBeNil)
			So(ids[1].Kind, ShouldEqual, "Test Foo")
		})
	})
}
