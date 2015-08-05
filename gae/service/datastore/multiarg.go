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
	valid bool

	getKey  func(nk newKeyFunc, slot reflect.Value) (Key, error)
	getPM   func(slot reflect.Value) (PropertyMap, error)
	setPM   func(slot reflect.Value, pm PropertyMap) error
	setKey  func(slot reflect.Value, k Key)
	newElem func() reflect.Value
}

func (mat *multiArgType) GetKeysPMs(nk newKeyFunc, slice reflect.Value) ([]Key, []PropertyMap, error) {
	retKey := make([]Key, slice.Len())
	retPM := make([]PropertyMap, slice.Len())
	lme := errors.LazyMultiError{Size: len(retKey)}
	for i := range retKey {
		key, err := mat.getKey(nk, slice.Index(i))
		if !lme.Assign(i, err) {
			retKey[i] = key
			pm, err := mat.getPM(slice.Index(i))
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
		return multiArgTypeInvalid()
	}
	return parseArg(e.Elem())
}

// parseArg checks that et is of type S, *S, I, P or *P, for some
// struct type S, for some interface type I, or some non-interface non-pointer
// type P such that P or *P implements PropertyLoadSaver.
func parseArg(et reflect.Type) multiArgType {
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
	return multiArgTypeInvalid()
}

type newKeyFunc func(kind, sid string, iid int64, par Key) Key

func multiArgTypeInvalid() multiArgType {
	return multiArgType{}
}

// multiArgTypePLS == []P
// *P implements PropertyLoadSaver
func multiArgTypePLS(et reflect.Type) multiArgType {
	ret := multiArgType{
		valid: true,

		getKey: func(nk newKeyFunc, slot reflect.Value) (Key, error) {
			return newKeyObjErr(nk, slot.Addr().Interface())
		},
		getPM: func(slot reflect.Value) (PropertyMap, error) {
			return slot.Addr().Interface().(PropertyLoadSaver).Save(true)
		},
		setPM: func(slot reflect.Value, pm PropertyMap) error {
			return slot.Addr().Interface().(PropertyLoadSaver).Load(pm)
		},
		setKey: func(slot reflect.Value, k Key) {
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
		valid: true,

		getKey: func(nk newKeyFunc, slot reflect.Value) (Key, error) {
			return newKeyObjErr(nk, slot.Interface())
		},
		getPM: func(slot reflect.Value) (PropertyMap, error) {
			return slot.Interface().(PropertyLoadSaver).Save(true)
		},
		setPM: func(slot reflect.Value, pm PropertyMap) error {
			return slot.Interface().(PropertyLoadSaver).Load(pm)
		},
		setKey: func(slot reflect.Value, k Key) {
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
	if cdc.problem != nil {
		return multiArgTypeInvalid()
	}
	toPLS := func(slot reflect.Value) PropertyLoadSaver {
		return &structPLS{slot, cdc}
	}
	return multiArgType{
		valid: true,

		getKey: func(nk newKeyFunc, slot reflect.Value) (Key, error) {
			return newKeyObjErr(nk, toPLS(slot))
		},
		getPM: func(slot reflect.Value) (PropertyMap, error) {
			return toPLS(slot).(PropertyLoadSaver).Save(true)
		},
		setPM: func(slot reflect.Value, pm PropertyMap) error {
			return toPLS(slot).(PropertyLoadSaver).Load(pm)
		},
		setKey: func(slot reflect.Value, k Key) {
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
	if cdc.problem != nil {
		return multiArgTypeInvalid()
	}
	toPLS := func(slot reflect.Value) PropertyLoadSaver {
		return &structPLS{slot.Elem(), cdc}
	}
	return multiArgType{
		valid: true,

		getKey: func(nk newKeyFunc, slot reflect.Value) (Key, error) {
			return newKeyObjErr(nk, toPLS(slot))
		},
		getPM: func(slot reflect.Value) (PropertyMap, error) {
			return toPLS(slot).(PropertyLoadSaver).Save(true)
		},
		setPM: func(slot reflect.Value, pm PropertyMap) error {
			return toPLS(slot).(PropertyLoadSaver).Load(pm)
		},
		setKey: func(slot reflect.Value, k Key) {
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
		valid: true,

		getKey: func(nk newKeyFunc, slot reflect.Value) (Key, error) {
			return newKeyObjErr(nk, slot.Elem().Interface())
		},
		getPM: func(slot reflect.Value) (PropertyMap, error) {
			pls, _ := mkPLSName(slot.Elem().Interface())
			return pls.Save(true)
		},
		setPM: func(slot reflect.Value, pm PropertyMap) error {
			pls, _ := mkPLSName(slot.Elem().Interface())
			return pls.Load(pm)
		},
		setKey: func(slot reflect.Value, k Key) {
			setKey(slot.Elem().Interface(), k)
		},
	}
}

func newKeyObjErr(nk newKeyFunc, src interface{}) (Key, error) {
	pls, name := mkPLSName(src)
	if key, _ := pls.GetMetaDefault("key", nil).(Key); key != nil {
		return key, nil
	}

	// get kind
	kind := pls.GetMetaDefault("kind", name).(string)
	if kind == "" {
		return nil, fmt.Errorf("unable to extract $kind from %v", src)
	}

	// get id - allow both to be default for default keys
	sid := pls.GetMetaDefault("id", "").(string)
	iid := pls.GetMetaDefault("id", 0).(int64)

	// get parent
	par, _ := pls.GetMetaDefault("parent", nil).(Key)

	return nk(kind, sid, iid, par), nil
}

func setKey(src interface{}, key Key) {
	pls, _ := mkPLSName(src)
	if pls.SetMeta("key", key) == ErrMetaFieldUnset {
		if key.StringID() != "" {
			pls.SetMeta("id", key.StringID())
		} else {
			pls.SetMeta("id", key.IntID())
		}
		pls.SetMeta("kind", key.Kind())
		pls.SetMeta("parent", key.Parent())
	}
}

func mkPLSName(o interface{}) (PropertyLoadSaver, string) {
	if pls, ok := o.(*structPLS); ok {
		return pls, pls.o.Type().Name()
	}
	if pls, ok := o.(PropertyLoadSaver); ok {
		return pls, ""
	}
	pls := GetPLS(o)
	name := pls.(*structPLS).o.Type().Name()
	return pls, name
}
