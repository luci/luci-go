// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

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

	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/common/errors"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func fakeDatastoreFactory(c context.Context, wantTxn bool) RawInterface {
	i := info.Get(c)
	return &fakeDatastore{
		aid: i.FullyQualifiedAppID(),
		ns:  i.GetNamespace(),
	}
}

type fakeDatastore struct {
	RawInterface
	aid string
	ns  string
}

func (f *fakeDatastore) mkKey(elems ...interface{}) *Key {
	return MakeKey(f.aid, f.ns, elems...)
}

func (f *fakeDatastore) Run(fq *FinalizedQuery, cb RawRunCB) error {
	lim, _ := fq.Limit()

	cursCB := func() (Cursor, error) {
		return fakeCursor("CURSOR"), nil
	}

	for i := int32(0); i < lim; i++ {
		if v, ok := fq.eqFilts["$err_single"]; ok {
			idx := fq.eqFilts["$err_single_idx"][0].Value().(int64)
			if idx == int64(i) {
				return errors.New(v[0].Value().(string))
			}
		}
		k := f.mkKey("Kind", i+1)
		if i == 10 {
			k = f.mkKey("Kind", "eleven")
		}
		pm := PropertyMap{"Value": {MkProperty(i)}}
		if err := cb(k, pm, cursCB); err != nil {
			if err == Stop {
				err = nil
			}
			return err
		}
	}
	return nil
}

func (f *fakeDatastore) PutMulti(keys []*Key, vals []PropertyMap, cb PutMultiCB) error {
	if keys[0].Kind() == "FailAll" {
		return errors.New("PutMulti fail all")
	}
	_, assertExtra := vals[0].GetMeta("assertExtra")
	for i, k := range keys {
		err := error(nil)
		if k.Kind() == "Fail" {
			err = errors.New("PutMulti fail")
		} else {
			So(vals[i]["Value"], ShouldResemble, []Property{MkProperty(i)})
			if assertExtra {
				So(vals[i]["Extra"], ShouldResemble, []Property{MkProperty("whoa")})
			}
			if k.Incomplete() {
				k = NewKey(k.AppID(), k.Namespace(), k.Kind(), "", int64(i+1), k.Parent())
			}
		}
		cb(k, err)
	}
	return nil
}

func (f *fakeDatastore) GetMulti(keys []*Key, _meta MultiMetaGetter, cb GetMultiCB) error {
	if keys[0].Kind() == "FailAll" {
		return errors.New("GetMulti fail all")
	}
	for i, k := range keys {
		if k.Kind() == "Fail" {
			cb(nil, errors.New("GetMulti fail"))
		} else if k.Kind() == "DNE" {
			cb(nil, ErrNoSuchEntity)
		} else {
			cb(PropertyMap{"Value": {MkProperty(i + 1)}}, nil)
		}
	}
	return nil
}

func (f *fakeDatastore) DeleteMulti(keys []*Key, cb DeleteMultiCB) error {
	if keys[0].Kind() == "FailAll" {
		return errors.New("DeleteMulti fail all")
	}
	for _, k := range keys {
		if k.Kind() == "Fail" {
			cb(errors.New("DeleteMulti fail"))
		} else {
			cb(nil)
		}
	}
	return nil
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

type permaBad struct {
	PropertyLoadSaver
}

func (f *permaBad) Load(pm PropertyMap) error {
	return errors.New("permaBad")
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
	f.Value = pm["Value"][0].Value().(int64)
	return nil
}

func (f *FakePLS) Save(withMeta bool) (PropertyMap, error) {
	if f.failSave {
		return nil, errors.New("FakePLS.Save")
	}
	ret := PropertyMap{
		"Value": {MkProperty(f.Value)},
		"Extra": {MkProperty("whoa")},
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

type MGSWithNoKind struct {
	S string
}

func (s *MGSWithNoKind) GetMeta(key string) (interface{}, bool) {
	return nil, false
}

func (s *MGSWithNoKind) GetAllMeta() PropertyMap {
	return PropertyMap{}
}

func (s *MGSWithNoKind) SetMeta(key string, val interface{}) bool {
	return false
}

var _ MetaGetterSetter = (*MGSWithNoKind)(nil)

func TestKeyForObj(t *testing.T) {
	t.Parallel()

	Convey("Test interface.KeyForObj", t, func() {
		c := info.Set(context.Background(), fakeInfo{})
		c = SetRawFactory(c, fakeDatastoreFactory)
		ds := Get(c)

		k := ds.MakeKey("Hello", "world")

		Convey("good", func() {
			Convey("struct containing $key", func() {
				type keyStruct struct {
					Key *Key `gae:"$key"`
				}

				ks := &keyStruct{k}
				So(ds.KeyForObj(ks), ShouldEqual, k)
			})

			Convey("struct containing default $id and $kind", func() {
				type idStruct struct {
					id  string `gae:"$id,wut"`
					knd string `gae:"$kind,SuperKind"`
				}

				So(ds.KeyForObj(&idStruct{}).String(), ShouldEqual, `s~aid:ns:/SuperKind,"wut"`)
			})

			Convey("struct containing $id and $parent", func() {
				So(ds.KeyForObj(&CommonStruct{ID: 4}).String(), ShouldEqual, `s~aid:ns:/CommonStruct,4`)

				So(ds.KeyForObj(&CommonStruct{ID: 4, Parent: k}).String(), ShouldEqual, `s~aid:ns:/Hello,"world"/CommonStruct,4`)
			})

			Convey("a propmap with $key", func() {
				pm := PropertyMap{}
				So(pm.SetMeta("key", k), ShouldBeTrue)
				So(ds.KeyForObj(pm).String(), ShouldEqual, `s~aid:ns:/Hello,"world"`)
			})

			Convey("a propmap with $id, $kind, $parent", func() {
				pm := PropertyMap{}
				So(pm.SetMeta("id", 100), ShouldBeTrue)
				So(pm.SetMeta("kind", "Sup"), ShouldBeTrue)
				So(ds.KeyForObj(pm).String(), ShouldEqual, `s~aid:ns:/Sup,100`)

				So(pm.SetMeta("parent", k), ShouldBeTrue)
				So(ds.KeyForObj(pm).String(), ShouldEqual, `s~aid:ns:/Hello,"world"/Sup,100`)
			})

			Convey("a pls with $id, $parent", func() {
				pls := GetPLS(&CommonStruct{ID: 1})
				So(ds.KeyForObj(pls).String(), ShouldEqual, `s~aid:ns:/CommonStruct,1`)

				So(pls.SetMeta("parent", k), ShouldBeTrue)
				So(ds.KeyForObj(pls).String(), ShouldEqual, `s~aid:ns:/Hello,"world"/CommonStruct,1`)
			})

			Convey("can see if things exist", func() {
				e, err := ds.Exists(k)
				So(err, ShouldBeNil)
				So(e, ShouldBeTrue)

				e, err = ds.Exists(ds.MakeKey("DNE", "nope"))
				So(err, ShouldBeNil)
				So(e, ShouldBeFalse)

				_, err = ds.Exists(ds.MakeKey("Fail", "boom"))
				So(err, ShouldErrLike, "GetMulti fail")
			})

		})

		Convey("bad", func() {
			Convey("a propmap without $kind", func() {
				pm := PropertyMap{}
				So(pm.SetMeta("id", 100), ShouldBeTrue)
				So(func() { ds.KeyForObj(pm) }, ShouldPanic)
			})

			Convey("a bad object", func() {
				type BadObj struct {
					ID int64 `gae:"$id"`

					NonSerializableField complex64
				}

				So(func() { ds.KeyForObjErr(&BadObj{ID: 1}) }, ShouldPanicLike,
					`field "NonSerializableField" has invalid type: complex64`)
			})
		})
	})
}

func TestPut(t *testing.T) {
	t.Parallel()

	Convey("Test Put/PutMulti", t, func() {
		c := info.Set(context.Background(), fakeInfo{})
		c = SetRawFactory(c, fakeDatastoreFactory)
		ds := Get(c)

		Convey("bad", func() {
			Convey("static can't serialize", func() {
				bss := []badStruct{{}, {}}
				So(func() { ds.PutMulti(bss) }, ShouldPanicLike,
					`field "Compy" has invalid type`)
			})

			Convey("static ptr can't serialize", func() {
				bss := []*badStruct{{}, {}}
				So(func() { ds.PutMulti(bss) }, ShouldPanicLike,
					`field "Compy" has invalid type: complex64`)
			})

			Convey("static bad type (non-slice)", func() {
				So(func() { ds.PutMulti(100) }, ShouldPanicLike,
					"invalid argument type: expected slice, got int")
			})

			Convey("static bad type (slice of bad type)", func() {
				So(func() { ds.PutMulti([]int{}) }, ShouldPanicLike,
					"invalid argument type: []int")
			})

			Convey("dynamic can't serialize", func() {
				fplss := []FakePLS{{failSave: true}, {}}
				So(ds.PutMulti(fplss), ShouldErrLike, "FakePLS.Save")
			})

			Convey("can't get keys", func() {
				fplss := []FakePLS{{failGetMeta: true}, {}}
				So(ds.PutMulti(fplss), ShouldErrLike, "unable to extract $kind")
			})

			Convey("get single error for RPC failure", func() {
				fplss := []FakePLS{{Kind: "FailAll"}, {}}
				So(ds.PutMulti(fplss), ShouldErrLike, "PutMulti fail all")
			})

			Convey("get multi error for individual failures", func() {
				fplss := []FakePLS{{}, {Kind: "Fail"}}
				So(ds.PutMulti(fplss), ShouldResemble, errors.MultiError{nil, errors.New("PutMulti fail")})
			})

			Convey("put with non-modifyable type is an error", func() {
				cs := CommonStruct{}
				So(func() { ds.Put(cs) }, ShouldPanicLike,
					"invalid Put input type (datastore.CommonStruct): not a pointer")
			})

			Convey("get with *Key is an error", func() {
				So(func() { ds.Get(&Key{}) }, ShouldPanicLike,
					"invalid Get input type (*datastore.Key): not user datatype")
			})

			Convey("struct with no $kind is an error", func() {
				s := MGSWithNoKind{}
				So(ds.Put(&s), ShouldErrLike, "unable to extract $kind")
			})

			Convey("struct with invalid but non-nil key is an error", func() {
				type BadParent struct {
					ID     int64 `gae:"$id"`
					Parent *Key  `gae:"$parent"`
				}
				// having an Incomplete parent makes an invalid key
				bp := &BadParent{ID: 1, Parent: ds.MakeKey("Something", 0)}
				So(ds.Put(bp), ShouldErrLike, ErrInvalidKey)
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
				So(ds.PutMulti(css), ShouldBeNil)
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
				So(ds.PutMulti(css), ShouldBeNil)
				for i, cs := range css {
					expect := int64(i + 1)
					if i == 4 {
						expect = 200
					}
					So(cs.ID, ShouldEqual, expect)
				}

				s := &CommonStruct{}
				So(ds.Put(s), ShouldBeNil)
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
				So(ds.PutMulti(fplss), ShouldBeNil)
				for i, fpls := range fplss {
					expect := int64(i + 1)
					if i == 4 {
						expect = 200
					}
					So(fpls.IntID, ShouldEqual, expect)
				}

				pm := PropertyMap{"Value": {MkProperty(0)}, "$kind": {MkPropertyNI("Pmap")}}
				So(ds.Put(pm), ShouldBeNil)
				So(ds.KeyForObj(pm).IntID(), ShouldEqual, 1)
			})

			Convey("[]P (map)", func() {
				pms := make([]PropertyMap, 7)
				for i := range pms {
					pms[i] = PropertyMap{
						"$kind": {MkProperty("Pmap")},
						"Value": {MkProperty(i)},
					}
					if i == 4 {
						So(pms[i].SetMeta("id", int64(200)), ShouldBeTrue)
					}
				}
				So(ds.PutMulti(pms), ShouldBeNil)
				for i, pm := range pms {
					expect := int64(i + 1)
					if i == 4 {
						expect = 200
					}
					So(ds.KeyForObj(pm).String(), ShouldEqual, fmt.Sprintf("s~aid:ns:/Pmap,%d", expect))
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
				So(ds.PutMulti(fplss), ShouldBeNil)
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
						"$kind": {MkProperty("Pmap")},
						"Value": {MkProperty(i)},
					}
					if i == 4 {
						So(pms[i].SetMeta("id", int64(200)), ShouldBeTrue)
					}
				}
				So(ds.PutMulti(pms), ShouldBeNil)
				for i, pm := range pms {
					expect := int64(i + 1)
					if i == 4 {
						expect = 200
					}
					So(ds.KeyForObj(*pm).String(), ShouldEqual, fmt.Sprintf("s~aid:ns:/Pmap,%d", expect))
				}
			})

			Convey("[]I", func() {
				ifs := []interface{}{
					&CommonStruct{Value: 0},
					&FakePLS{Value: 1},
					PropertyMap{"Value": {MkProperty(2)}, "$kind": {MkPropertyNI("Pmap")}},
					&PropertyMap{"Value": {MkProperty(3)}, "$kind": {MkPropertyNI("Pmap")}},
				}
				So(ds.PutMulti(ifs), ShouldBeNil)
				for i := range ifs {
					switch i {
					case 0:
						So(ifs[i].(*CommonStruct).ID, ShouldEqual, 1)
					case 1:
						fpls := ifs[i].(*FakePLS)
						So(fpls.IntID, ShouldEqual, 2)
					case 2:
						So(ds.KeyForObj(ifs[i].(PropertyMap)).String(), ShouldEqual, "s~aid:ns:/Pmap,3")
					case 3:
						So(ds.KeyForObj(*ifs[i].(*PropertyMap)).String(), ShouldEqual, "s~aid:ns:/Pmap,4")
					}
				}
			})

		})

	})
}

func TestDelete(t *testing.T) {
	t.Parallel()

	Convey("Test Delete/DeleteMulti", t, func() {
		c := info.Set(context.Background(), fakeInfo{})
		c = SetRawFactory(c, fakeDatastoreFactory)
		ds := Get(c)
		So(ds, ShouldNotBeNil)

		Convey("bad", func() {
			Convey("get single error for RPC failure", func() {
				keys := []*Key{
					MakeKey("s~aid", "ns", "FailAll", 1),
					MakeKey("s~aid", "ns", "Ok", 1),
				}
				So(ds.DeleteMulti(keys).Error(), ShouldEqual, "DeleteMulti fail all")
			})

			Convey("get multi error for individual failure", func() {
				keys := []*Key{
					ds.MakeKey("Ok", 1),
					ds.MakeKey("Fail", 2),
				}
				So(ds.DeleteMulti(keys).Error(), ShouldEqual, "DeleteMulti fail")
			})

			Convey("get single error when deleting a single", func() {
				k := ds.MakeKey("Fail", 1)
				So(ds.Delete(k).Error(), ShouldEqual, "DeleteMulti fail")
			})
		})

	})
}

func TestGet(t *testing.T) {
	t.Parallel()

	Convey("Test Get/GetMulti", t, func() {
		c := info.Set(context.Background(), fakeInfo{})
		c = SetRawFactory(c, fakeDatastoreFactory)
		ds := Get(c)
		So(ds, ShouldNotBeNil)

		Convey("bad", func() {
			Convey("static can't serialize", func() {
				toGet := []badStruct{{}, {}}
				So(func() { ds.GetMulti(toGet) }, ShouldPanicLike,
					`field "Compy" has invalid type: complex64`)
			})

			Convey("can't get keys", func() {
				fplss := []FakePLS{{failGetMeta: true}, {}}
				So(ds.GetMulti(fplss), ShouldErrLike, "unable to extract $kind")
			})

			Convey("get single error for RPC failure", func() {
				fplss := []FakePLS{
					{IntID: 1, Kind: "FailAll"},
					{IntID: 2},
				}
				So(ds.GetMulti(fplss).Error(), ShouldEqual, "GetMulti fail all")
			})

			Convey("get multi error for individual failures", func() {
				fplss := []FakePLS{{IntID: 1}, {IntID: 2, Kind: "Fail"}}
				So(ds.GetMulti(fplss), ShouldResemble, errors.MultiError{nil, errors.New("GetMulti fail")})
			})

			Convey("get with non-modifiable type is an error", func() {
				cs := CommonStruct{}
				So(func() { ds.Get(cs) }, ShouldPanicLike,
					"invalid Get input type (datastore.CommonStruct): not a pointer")
			})

			Convey("get with nil is an error", func() {
				So(func() { ds.Get(nil) }, ShouldPanicLike,
					"invalid Get input type (<nil>): no type information")
			})

			Convey("get with ptr-to-nonstruct is an error", func() {
				val := 100
				So(func() { ds.Get(&val) }, ShouldPanicLike,
					"invalid Get input type (*int): does not point to a struct")
			})

			Convey("failure to save metadata is no problem though", func() {
				// It just won't save the key
				cs := &FakePLS{IntID: 10, failSetMeta: true}
				So(ds.Get(cs), ShouldBeNil)
			})
		})

		Convey("ok", func() {
			Convey("Get", func() {
				cs := &CommonStruct{ID: 1}
				So(ds.Get(cs), ShouldBeNil)
				So(cs.Value, ShouldEqual, 1)
			})

			Convey("Raw access too", func() {
				rds := ds.Raw()
				keys := []*Key{ds.MakeKey("Kind", 1)}
				So(rds.GetMulti(keys, nil, func(pm PropertyMap, err error) error {
					So(err, ShouldBeNil)
					So(pm["Value"][0].Value(), ShouldEqual, 1)
					return nil
				}), ShouldBeNil)
			})

			Convey("but general failure to save is fine on a Get", func() {
				cs := &FakePLS{failSave: true, IntID: 7}
				So(ds.Get(cs), ShouldBeNil)
			})
		})

	})
}

func TestGetAll(t *testing.T) {
	t.Parallel()

	Convey("Test GetAll", t, func() {
		c := info.Set(context.Background(), fakeInfo{})
		c = SetRawFactory(c, fakeDatastoreFactory)
		ds := Get(c)
		So(ds, ShouldNotBeNil)

		q := NewQuery("").Limit(5)

		Convey("bad", func() {
			Convey("nil target", func() {
				So(func() { ds.GetAll(q, (*[]PropertyMap)(nil)) }, ShouldPanicLike,
					"invalid GetAll dst: <nil>")
			})

			Convey("bad type", func() {
				output := 100
				So(func() { ds.GetAll(q, &output) }, ShouldPanicLike,
					"invalid argument type: expected slice, got int")
			})

			Convey("bad type (non pointer)", func() {
				So(func() { ds.GetAll(q, "moo") }, ShouldPanicLike,
					"invalid GetAll dst: must have a ptr-to-slice")
			})

			Convey("bad type (underspecified)", func() {
				output := []PropertyLoadSaver(nil)
				So(func() { ds.GetAll(q, &output) }, ShouldPanicLike,
					"invalid GetAll dst (non-concrete element type): *[]datastore.PropertyLoadSaver")
			})
		})

		Convey("ok", func() {
			Convey("*[]S", func() {
				output := []CommonStruct(nil)
				So(ds.GetAll(q, &output), ShouldBeNil)
				So(len(output), ShouldEqual, 5)
				for i, o := range output {
					So(o.ID, ShouldEqual, i+1)
					So(o.Value, ShouldEqual, i)
				}
			})

			Convey("*[]*S", func() {
				output := []*CommonStruct(nil)
				So(ds.GetAll(q, &output), ShouldBeNil)
				So(len(output), ShouldEqual, 5)
				for i, o := range output {
					So(o.ID, ShouldEqual, i+1)
					So(o.Value, ShouldEqual, i)
				}
			})

			Convey("*[]P", func() {
				output := []FakePLS(nil)
				So(ds.GetAll(q, &output), ShouldBeNil)
				So(len(output), ShouldEqual, 5)
				for i, o := range output {
					So(o.gotLoaded, ShouldBeTrue)
					So(o.IntID, ShouldEqual, i+1)
					So(o.Value, ShouldEqual, i)
				}
			})

			Convey("*[]P (map)", func() {
				output := []PropertyMap(nil)
				So(ds.GetAll(q, &output), ShouldBeNil)
				So(len(output), ShouldEqual, 5)
				for i, o := range output {
					k, ok := o.GetMeta("key")
					So(ok, ShouldBeTrue)
					So(k.(*Key).IntID(), ShouldEqual, i+1)
					So(o["Value"][0].Value().(int64), ShouldEqual, i)
				}
			})

			Convey("*[]*P", func() {
				output := []*FakePLS(nil)
				So(ds.GetAll(q, &output), ShouldBeNil)
				So(len(output), ShouldEqual, 5)
				for i, o := range output {
					So(o.gotLoaded, ShouldBeTrue)
					So(o.IntID, ShouldEqual, i+1)
					So(o.Value, ShouldEqual, i)
				}
			})

			Convey("*[]*P (map)", func() {
				output := []*PropertyMap(nil)
				So(ds.GetAll(q, &output), ShouldBeNil)
				So(len(output), ShouldEqual, 5)
				for i, op := range output {
					o := *op
					k, ok := o.GetMeta("key")
					So(ok, ShouldBeTrue)
					So(k.(*Key).IntID(), ShouldEqual, i+1)
					So(o["Value"][0].Value().(int64), ShouldEqual, i)
				}
			})

			Convey("*[]*Key", func() {
				output := []*Key(nil)
				So(ds.GetAll(q, &output), ShouldBeNil)
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
		c = SetRawFactory(c, fakeDatastoreFactory)
		ds := Get(c)
		So(ds, ShouldNotBeNil)

		q := NewQuery("kind").Limit(5)

		Convey("bad", func() {
			assertBadTypePanics := func(cb interface{}) {
				So(func() { ds.Run(q, cb) }, ShouldPanicLike,
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
				So(func() { ds.Run(q, cb) }, ShouldPanicLike,
					"invalid argument type: int")
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
				So(ds.Run(q, func(c CommonStruct) {
					i++
				}), ShouldErrLike, "Query fail")
				So(i, ShouldEqual, 3)
			})

			Convey("return error on serialization failure", func() {
				So(ds.Run(q, func(_ permaBad) {
					panic("never here")
				}).Error(), ShouldEqual, "permaBad")
			})
		})

		Convey("ok", func() {
			Convey("can return error to stop", func() {
				i := 0
				So(ds.Run(q, func(c CommonStruct) error {
					i++
					return Stop
				}), ShouldBeNil)
				So(i, ShouldEqual, 1)

				i = 0
				So(ds.Run(q, func(c CommonStruct, _ CursorCB) error {
					i++
					return fmt.Errorf("my error")
				}), ShouldErrLike, "my error")
				So(i, ShouldEqual, 1)
			})

			Convey("Can optionally get cursor function", func() {
				i := 0
				So(ds.Run(q, func(c CommonStruct, ccb CursorCB) {
					i++
					curs, err := ccb()
					So(err, ShouldBeNil)
					So(curs.String(), ShouldEqual, "CURSOR")
				}), ShouldBeNil)
				So(i, ShouldEqual, 5)
			})

			Convey("*S", func() {
				i := 0
				So(ds.Run(q, func(cs *CommonStruct) {
					So(cs.ID, ShouldEqual, i+1)
					So(cs.Value, ShouldEqual, i)
					i++
				}), ShouldBeNil)
			})

			Convey("*P", func() {
				i := 0
				So(ds.Run(q.Limit(12), func(fpls *FakePLS) {
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
				So(ds.Run(q, func(pm *PropertyMap) {
					k, ok := pm.GetMeta("key")
					So(ok, ShouldBeTrue)
					So(k.(*Key).IntID(), ShouldEqual, i+1)
					So((*pm)["Value"][0].Value(), ShouldEqual, i)
					i++
				}), ShouldBeNil)
			})

			Convey("S", func() {
				i := 0
				So(ds.Run(q, func(cs CommonStruct) {
					So(cs.ID, ShouldEqual, i+1)
					So(cs.Value, ShouldEqual, i)
					i++
				}), ShouldBeNil)
			})

			Convey("P", func() {
				i := 0
				So(ds.Run(q, func(fpls FakePLS) {
					So(fpls.gotLoaded, ShouldBeTrue)
					So(fpls.IntID, ShouldEqual, i+1)
					So(fpls.Value, ShouldEqual, i)
					i++
				}), ShouldBeNil)
			})

			Convey("P (map)", func() {
				i := 0
				So(ds.Run(q, func(pm PropertyMap) {
					k, ok := pm.GetMeta("key")
					So(ok, ShouldBeTrue)
					So(k.(*Key).IntID(), ShouldEqual, i+1)
					So(pm["Value"][0].Value(), ShouldEqual, i)
					i++
				}), ShouldBeNil)
			})

			Convey("Key", func() {
				i := 0
				So(ds.Run(q, func(k *Key) {
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
	for _, k := range keys {
		data, ok := d.data[k.String()]
		if ok {
			cb(data, nil)
		} else {
			cb(nil, ErrNoSuchEntity)
		}
	}
	return nil
}

func (d *fixedDataDatastore) PutMulti(keys []*Key, vals []PropertyMap, cb PutMultiCB) error {
	if d.data == nil {
		d.data = make(map[string]PropertyMap, len(keys))
	}
	for i, k := range keys {
		if k.Incomplete() {
			panic("key is incomplete, don't do that.")
		}
		d.data[k.String()], _ = vals[i].Save(false)
		cb(k, nil)
	}
	return nil
}

func TestSchemaChange(t *testing.T) {
	t.Parallel()

	Convey("Test changing schemas", t, func() {
		fds := fixedDataDatastore{}
		ds := &datastoreImpl{&fds, "", ""}

		Convey("Can add fields", func() {
			initial := PropertyMap{
				"$key": {mpNI(ds.MakeKey("Val", 10))},
				"Val":  {mp(100)},
			}
			So(ds.Put(initial), ShouldBeNil)

			type Val struct {
				ID int64 `gae:"$id"`

				Val    int64
				TwoVal int64 // whoa, TWO vals! amazing
			}
			tv := &Val{ID: 10, TwoVal: 2}
			So(ds.Get(tv), ShouldBeNil)
			So(tv, ShouldResemble, &Val{ID: 10, Val: 100, TwoVal: 2})
		})

		Convey("Removing fields", func() {
			initial := PropertyMap{
				"$key":   {mpNI(ds.MakeKey("Val", 10))},
				"Val":    {mp(100)},
				"TwoVal": {mp(200)},
			}
			So(ds.Put(initial), ShouldBeNil)

			Convey("is normally an error", func() {
				type Val struct {
					ID int64 `gae:"$id"`

					Val int64
				}
				tv := &Val{ID: 10}
				So(ds.Get(tv), ShouldErrLike,
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
				So(ds.Get(tv), ShouldBeNil)
				So(tv, ShouldResembleV, &Val{
					ID:  10,
					Val: 100,
					Extra: PropertyMap{
						"TwoVal": {mp(200)},
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
				"Hello": {mp("Hello")},
				"World": {mp(true)},
			}}
			So(ds.Put(ex), ShouldBeNil)

			ex = &Expando{ID: 10}
			So(ds.Get(ex), ShouldBeNil)
			So(ex, ShouldResembleV, &Expando{
				ID:        10,
				Something: 17,
				Extra: PropertyMap{
					"Hello": {mp("Hello")},
					"World": {mp(true)},
				},
			})
		})

		Convey("Can read-but-not-write", func() {
			initial := PropertyMap{
				"$key":   {mpNI(ds.MakeKey("Convert", 10))},
				"Val":    {mp(100)},
				"TwoVal": {mp(200)},
			}
			So(ds.Put(initial), ShouldBeNil)
			type Convert struct {
				ID int64 `gae:"$id"`

				Val    int64
				NewVal int64
				Extra  PropertyMap `gae:"-,extra"`
			}
			c := &Convert{ID: 10}
			So(ds.Get(c), ShouldBeNil)
			So(c, ShouldResembleV, &Convert{
				ID: 10, Val: 100, NewVal: 0, Extra: PropertyMap{"TwoVal": {mp(200)}},
			})
			c.NewVal = c.Extra["TwoVal"][0].Value().(int64)
			So(ds.Put(c), ShouldBeNil)

			c = &Convert{ID: 10}
			So(ds.Get(c), ShouldBeNil)
			So(c, ShouldResembleV, &Convert{
				ID: 10, Val: 100, NewVal: 200, Extra: nil,
			})
		})

		Convey("Can black hole", func() {
			initial := PropertyMap{
				"$key":   {mpNI(ds.MakeKey("BlackHole", 10))},
				"Val":    {mp(100)},
				"TwoVal": {mp(200)},
			}
			So(ds.Put(initial), ShouldBeNil)
			type BlackHole struct {
				ID int64 `gae:"$id"`

				NewStuff  string
				blackHole PropertyMap `gae:"-,extra"`
			}
			b := &BlackHole{ID: 10, NewStuff: "(╯°□°)╯︵ ┻━┻"}
			So(ds.Get(b), ShouldBeNil)
			So(b, ShouldResemble, &BlackHole{ID: 10, NewStuff: "(╯°□°)╯︵ ┻━┻"})
		})

		Convey("Can change field types", func() {
			initial := PropertyMap{
				"$key": {mpNI(ds.MakeKey("IntChange", 10))},
				"Val":  {mp(100)},
			}
			So(ds.Put(initial), ShouldBeNil)

			type IntChange struct {
				ID    int64 `gae:"$id"`
				Val   string
				Extra PropertyMap `gae:"-,extra"`
			}
			i := &IntChange{ID: 10}
			So(ds.Get(i), ShouldBeNil)
			So(i, ShouldResembleV, &IntChange{ID: 10, Extra: PropertyMap{"Val": {mp(100)}}})
			i.Val = fmt.Sprint(i.Extra["Val"][0].Value())
			So(ds.Put(i), ShouldBeNil)

			i = &IntChange{ID: 10}
			So(ds.Get(i), ShouldBeNil)
			So(i, ShouldResembleV, &IntChange{ID: 10, Val: "100"})
		})

		Convey("Native fields have priority over Extra fields", func() {
			type Dup struct {
				ID    int64 `gae:"$id"`
				Val   int64
				Extra PropertyMap `gae:",extra"`
			}
			d := &Dup{ID: 10, Val: 100, Extra: PropertyMap{
				"Val":   {mp(200)},
				"Other": {mp("other")},
			}}
			So(ds.Put(d), ShouldBeNil)

			d = &Dup{ID: 10}
			So(ds.Get(d), ShouldBeNil)
			So(d, ShouldResembleV, &Dup{
				ID: 10, Val: 100, Extra: PropertyMap{"Other": {mp("other")}},
			})
		})

		Convey("Can change repeated field to non-repeating field", func() {
			initial := PropertyMap{
				"$key": {mpNI(ds.MakeKey("NonRepeating", 10))},
				"Val":  {mp(100), mp(200), mp(400)},
			}
			So(ds.Put(initial), ShouldBeNil)

			type NonRepeating struct {
				ID    int64 `gae:"$id"`
				Val   int64
				Extra PropertyMap `gae:",extra"`
			}
			n := &NonRepeating{ID: 10}
			So(ds.Get(n), ShouldBeNil)
			So(n, ShouldResembleV, &NonRepeating{
				ID: 10, Val: 0, Extra: PropertyMap{
					"Val": {mp(100), mp(200), mp(400)},
				},
			})
		})

		Convey("Deals correctly with recursive types", func() {
			initial := PropertyMap{
				"$key": {mpNI(ds.MakeKey("Outer", 10))},
				"I.A":  {mp(1), mp(2), mp(4)},
				"I.B":  {mp(10), mp(20), mp(40)},
				"I.C":  {mp(100), mp(200), mp(400)},
			}
			So(ds.Put(initial), ShouldBeNil)
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
			So(ds.Get(o), ShouldBeNil)
			So(o, ShouldResembleV, &Outer{
				ID: 10,
				I: []Inner{
					{1, 10},
					{2, 20},
					{4, 40},
				},
				Extra: PropertyMap{
					"I.C": {mp(100), mp(200), mp(400)},
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
		So(ids, ShouldResembleV, expected)
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
