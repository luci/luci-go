// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package datastore

import (
	"fmt"
	"reflect"

	"github.com/luci/luci-go/common/errors"
)

type multiArgType struct {
	getKey    func(aid, ns string, slot reflect.Value) (*Key, error)
	getPM     func(slot reflect.Value) (PropertyMap, error)
	getMetaPM func(slot reflect.Value) PropertyMap
	setPM     func(slot reflect.Value, pm PropertyMap) error
	setKey    func(slot reflect.Value, k *Key)
	newElem   func() reflect.Value
}

func (mat *multiArgType) GetKeysPMs(aid, ns string, slice reflect.Value, meta bool) ([]*Key, []PropertyMap, error) {
	retKey := make([]*Key, slice.Len())
	retPM := make([]PropertyMap, slice.Len())
	getter := mat.getPM
	if meta {
		getter = func(slot reflect.Value) (PropertyMap, error) {
			return mat.getMetaPM(slot), nil
		}
	}
	lme := errors.NewLazyMultiError(len(retKey))
	for i := range retKey {
		key, err := mat.getKey(aid, ns, slice.Index(i))
		if !lme.Assign(i, err) {
			retKey[i] = key
			pm, err := getter(slice.Index(i))
			if !lme.Assign(i, err) {
				retPM[i] = pm
			}
		}
	}
	return retKey, retPM, lme.Get()
}

// parseMultiArg checks that v has type []S, []*S, []I, []P or []*P, for some
// struct type S, for some interface type I, or some non-interface non-pointer
// type P such that P or *P implements PropertyLoadSaver.
func parseMultiArg(e reflect.Type) multiArgType {
	if e.Kind() != reflect.Slice {
		panic(fmt.Errorf("invalid argument type: expected slice, got %s", e))
	}
	return parseArg(e.Elem(), true)
}

// parseArg checks that et is of type S, *S, I, P or *P, for some
// struct type S, for some interface type I, or some non-interface non-pointer
// type P such that P or *P implements PropertyLoadSaver.
func parseArg(et reflect.Type, multi bool) multiArgType {
	if reflect.PtrTo(et).Implements(typeOfPropertyLoadSaver) {
		return multiArgTypePLS(et)
	}
	if et.Implements(typeOfPropertyLoadSaver) && et.Kind() != reflect.Interface {
		return multiArgTypePLSPtr(et.Elem())
	}
	switch et.Kind() {
	case reflect.Struct:
		return multiArgTypeStruct(et)
	case reflect.Interface:
		return multiArgTypeInterface()
	case reflect.Ptr:
		et = et.Elem()
		if et.Kind() == reflect.Struct {
			return multiArgTypeStructPtr(et)
		}
	}
	if multi {
		panic(fmt.Errorf("invalid argument type: []%s", et))
	}
	panic(fmt.Errorf("invalid argument type: %s", et))
}

type newKeyFunc func(kind, sid string, iid int64, par Key) Key

// multiArgTypePLS == []P
// *P implements PropertyLoadSaver
func multiArgTypePLS(et reflect.Type) multiArgType {
	ret := multiArgType{
		getKey: func(aid, ns string, slot reflect.Value) (*Key, error) {
			return newKeyObjErr(aid, ns, slot.Addr().Interface())
		},
		getPM: func(slot reflect.Value) (PropertyMap, error) {
			return slot.Addr().Interface().(PropertyLoadSaver).Save(true)
		},
		getMetaPM: func(slot reflect.Value) PropertyMap {
			return getMGS(slot.Addr().Interface()).GetAllMeta()
		},
		setPM: func(slot reflect.Value, pm PropertyMap) error {
			return slot.Addr().Interface().(PropertyLoadSaver).Load(pm)
		},
		setKey: func(slot reflect.Value, k *Key) {
			setKey(slot.Addr().Interface(), k)
		},
	}
	if et.Kind() == reflect.Map {
		ret.newElem = func() reflect.Value {
			// Create a *map so that way slot.Addr() works above when this is
			// called from Run(). Otherwise the map is 'unaddressable' according
			// to reflect.  ¯\_(ツ)_/¯
			ptr := reflect.New(et)
			ptr.Elem().Set(reflect.MakeMap(et))
			return ptr.Elem()
		}
	} else {
		ret.newElem = func() reflect.Value {
			return reflect.New(et).Elem()
		}
	}
	return ret
}

// multiArgTypePLSPtr == []*P
// *P implements PropertyLoadSaver
func multiArgTypePLSPtr(et reflect.Type) multiArgType {
	ret := multiArgType{
		getKey: func(aid, ns string, slot reflect.Value) (*Key, error) {
			return newKeyObjErr(aid, ns, slot.Interface())
		},
		getPM: func(slot reflect.Value) (PropertyMap, error) {
			return slot.Interface().(PropertyLoadSaver).Save(true)
		},
		getMetaPM: func(slot reflect.Value) PropertyMap {
			return getMGS(slot.Interface()).GetAllMeta()
		},
		setPM: func(slot reflect.Value, pm PropertyMap) error {
			return slot.Interface().(PropertyLoadSaver).Load(pm)
		},
		setKey: func(slot reflect.Value, k *Key) {
			setKey(slot.Interface(), k)
		},
	}
	if et.Kind() == reflect.Map {
		ret.newElem = func() reflect.Value {
			ptr := reflect.New(et)
			ptr.Elem().Set(reflect.MakeMap(et))
			return ptr
		}
	} else {
		ret.newElem = func() reflect.Value { return reflect.New(et) }
	}
	return ret
}

// multiArgTypeStruct == []S
func multiArgTypeStruct(et reflect.Type) multiArgType {
	cdc := getCodec(et)
	toPLS := func(slot reflect.Value) *structPLS {
		return &structPLS{slot, cdc}
	}
	return multiArgType{
		getKey: func(aid, ns string, slot reflect.Value) (*Key, error) {
			return newKeyObjErr(aid, ns, toPLS(slot))
		},
		getPM: func(slot reflect.Value) (PropertyMap, error) {
			return toPLS(slot).Save(true)
		},
		getMetaPM: func(slot reflect.Value) PropertyMap {
			if slot.Type().Implements(typeOfMGS) {
				return slot.Interface().(MetaGetterSetter).GetAllMeta()
			}
			return toPLS(slot).GetAllMeta()
		},
		setPM: func(slot reflect.Value, pm PropertyMap) error {
			return toPLS(slot).Load(pm)
		},
		setKey: func(slot reflect.Value, k *Key) {
			setKey(toPLS(slot), k)
		},
		newElem: func() reflect.Value {
			return reflect.New(et).Elem()
		},
	}
}

// multiArgTypeStructPtr == []*S
func multiArgTypeStructPtr(et reflect.Type) multiArgType {
	cdc := getCodec(et)
	toPLS := func(slot reflect.Value) *structPLS {
		return &structPLS{slot.Elem(), cdc}
	}
	return multiArgType{
		getKey: func(aid, ns string, slot reflect.Value) (*Key, error) {
			return newKeyObjErr(aid, ns, toPLS(slot))
		},
		getPM: func(slot reflect.Value) (PropertyMap, error) {
			return toPLS(slot).Save(true)
		},
		getMetaPM: func(slot reflect.Value) PropertyMap {
			if slot.Elem().Type().Implements(typeOfMGS) {
				return getMGS(slot.Interface()).GetAllMeta()
			}
			return toPLS(slot).GetAllMeta()
		},
		setPM: func(slot reflect.Value, pm PropertyMap) error {
			return toPLS(slot).Load(pm)
		},
		setKey: func(slot reflect.Value, k *Key) {
			setKey(toPLS(slot), k)
		},
		newElem: func() reflect.Value {
			return reflect.New(et)
		},
	}
}

// multiArgTypeInterface == []I
func multiArgTypeInterface() multiArgType {
	return multiArgType{
		getKey: func(aid, ns string, slot reflect.Value) (*Key, error) {
			return newKeyObjErr(aid, ns, slot.Elem().Interface())
		},
		getPM: func(slot reflect.Value) (PropertyMap, error) {
			return mkPLS(slot.Elem().Interface()).Save(true)
		},
		getMetaPM: func(slot reflect.Value) PropertyMap {
			return getMGS(slot.Elem().Interface()).GetAllMeta()
		},
		setPM: func(slot reflect.Value, pm PropertyMap) error {
			return mkPLS(slot.Elem().Interface()).Load(pm)
		},
		setKey: func(slot reflect.Value, k *Key) {
			setKey(slot.Elem().Interface(), k)
		},
	}
}

func newKeyObjErr(aid, ns string, src interface{}) (*Key, error) {
	pls := getMGS(src)
	if key, _ := GetMetaDefault(pls, "key", nil).(*Key); key != nil {
		return key, nil
	}

	// get kind
	kind := GetMetaDefault(pls, "kind", "").(string)
	if kind == "" {
		return nil, fmt.Errorf("unable to extract $kind from %T", src)
	}

	// get id - allow both to be default for default keys
	sid := GetMetaDefault(pls, "id", "").(string)
	iid := GetMetaDefault(pls, "id", 0).(int64)

	// get parent
	par, _ := GetMetaDefault(pls, "parent", nil).(*Key)

	return NewKey(aid, ns, kind, sid, iid, par), nil
}

func setKey(src interface{}, key *Key) {
	pls := getMGS(src)
	if !pls.SetMeta("key", key) {
		lst := key.LastTok()
		if lst.StringID != "" {
			pls.SetMeta("id", lst.StringID)
		} else {
			pls.SetMeta("id", lst.IntID)
		}
		pls.SetMeta("kind", lst.Kind)
		pls.SetMeta("parent", key.Parent())
	}
}

func mkPLS(o interface{}) PropertyLoadSaver {
	if pls, ok := o.(PropertyLoadSaver); ok {
		return pls
	}
	return GetPLS(o)
}
