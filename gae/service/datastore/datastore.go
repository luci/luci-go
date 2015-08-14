// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package datastore

import (
	"fmt"
	"reflect"

	"github.com/luci/luci-go/common/errors"
)

type datastoreImpl struct{ RawInterface }

var _ Interface = (*datastoreImpl)(nil)

func (d *datastoreImpl) KeyForObj(src interface{}) Key {
	ret, err := d.KeyForObjErr(src)
	if err != nil {
		panic(err)
	}
	return ret
}

func (d *datastoreImpl) KeyForObjErr(src interface{}) (Key, error) {
	return newKeyObjErr(d.NewKey, src)
}

func (d *datastoreImpl) Run(q Query, cbIface interface{}) error {
	// TODO(riannucci): Profile and determine if any of this is causing a real
	// slowdown. Could potentially cache reflection stuff by cbType?
	cbTyp := reflect.TypeOf(cbIface)

	badSig := false
	mat := multiArgType{}
	isKey := false

	if cbTyp.Kind() == reflect.Func && cbTyp.NumIn() == 2 && cbTyp.NumOut() == 1 {
		firstArg := cbTyp.In(0)
		if firstArg == typeOfKey {
			isKey = true
		} else {
			mat = parseArg(firstArg)
			badSig = !mat.valid || mat.newElem == nil
		}
	} else {
		badSig = true
	}

	if badSig || cbTyp.Out(0) != typeOfBool || cbTyp.In(1) != typeOfCursorCB {
		panic(fmt.Errorf(
			"cb does not match the required callback signature: `%T` != `func(TYPE, CursorCB) bool`",
			cbIface))
	}

	if isKey {
		cb := cbIface.(func(Key, CursorCB) bool)
		return d.RawInterface.Run(q.KeysOnly(), func(k Key, _ PropertyMap, gc CursorCB) bool {
			return cb(k, gc)
		})
	}

	cbVal := reflect.ValueOf(cbIface)

	innerErr := error(nil)
	err := d.RawInterface.Run(q, func(k Key, pm PropertyMap, gc CursorCB) bool {
		itm := mat.newElem()
		if innerErr = mat.setPM(itm, pm); innerErr != nil {
			return false
		}
		mat.setKey(itm, k)
		return cbVal.Call([]reflect.Value{itm, reflect.ValueOf(gc)})[0].Bool()
	})
	if err == nil {
		err = innerErr
	}
	return err
}

func (d *datastoreImpl) GetAll(q Query, dst interface{}) error {
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Ptr {
		return fmt.Errorf("invalid GetAll dst: must have a ptr-to-slice: %T", dst)
	}
	if !v.IsValid() || v.IsNil() {
		return errors.New("invalid GetAll dst: <nil>")
	}

	if keys, ok := dst.(*[]Key); ok {
		return d.RawInterface.Run(q.KeysOnly(), func(k Key, _ PropertyMap, _ CursorCB) bool {
			*keys = append(*keys, k)
			return true
		})
	}

	slice := v.Elem()
	mat := parseMultiArg(slice.Type())
	if !mat.valid || mat.newElem == nil {
		return fmt.Errorf("invalid GetAll input type: %T", dst)
	}

	lme := errors.NewLazyMultiError(slice.Len())
	i := 0
	err := d.RawInterface.Run(q, func(k Key, pm PropertyMap, _ CursorCB) bool {
		slice.Set(reflect.Append(slice, mat.newElem()))
		itm := slice.Index(i)
		mat.setKey(itm, k)
		lme.Assign(i, mat.setPM(itm, pm))
		i++
		return true
	})
	if err == nil {
		err = lme.Get()
	}
	return err
}

func isOkType(v reflect.Type) bool {
	if v.Implements(typeOfPropertyLoadSaver) {
		return true
	}
	if v.Kind() == reflect.Ptr && v.Elem().Kind() == reflect.Struct {
		return true
	}
	return false
}

func (d *datastoreImpl) Get(dst interface{}) (err error) {
	if !isOkType(reflect.TypeOf(dst)) {
		return fmt.Errorf("invalid Get input type: %T", dst)
	}
	return errors.SingleError(d.GetMulti([]interface{}{dst}))
}

func (d *datastoreImpl) Put(src interface{}) (err error) {
	if !isOkType(reflect.TypeOf(src)) {
		return fmt.Errorf("invalid Put input type: %T", src)
	}
	return errors.SingleError(d.PutMulti([]interface{}{src}))
}

func (d *datastoreImpl) Delete(key Key) (err error) {
	return errors.SingleError(d.DeleteMulti([]Key{key}))
}

func (d *datastoreImpl) GetMulti(dst interface{}) error {
	slice := reflect.ValueOf(dst)
	mat := parseMultiArg(slice.Type())
	if !mat.valid {
		return fmt.Errorf("invalid GetMulti input type: %T", dst)
	}

	keys, pms, err := mat.GetKeysPMs(d.NewKey, slice)
	if err != nil {
		return err
	}

	lme := errors.NewLazyMultiError(len(keys))
	i := 0
	meta := NewMultiMetaGetter(pms)
	err = d.RawInterface.GetMulti(keys, meta, func(pm PropertyMap, err error) {
		if !lme.Assign(i, err) {
			lme.Assign(i, mat.setPM(slice.Index(i), pm))
		}
		i++
	})

	if err == nil {
		err = lme.Get()
	}
	return err
}

func (d *datastoreImpl) PutMulti(src interface{}) error {
	slice := reflect.ValueOf(src)
	mat := parseMultiArg(slice.Type())
	if !mat.valid {
		return fmt.Errorf("invalid PutMulti input type: %T", src)
	}

	keys, vals, err := mat.GetKeysPMs(d.NewKey, slice)
	if err != nil {
		return err
	}

	lme := errors.NewLazyMultiError(len(keys))
	i := 0
	err = d.RawInterface.PutMulti(keys, vals, func(key Key, err error) {
		if key != keys[i] {
			mat.setKey(slice.Index(i), key)
		}
		lme.Assign(i, err)
		i++
	})

	if err == nil {
		err = lme.Get()
	}
	return err
}

func (d *datastoreImpl) DeleteMulti(keys []Key) (err error) {
	lme := errors.NewLazyMultiError(len(keys))
	i := 0
	extErr := d.RawInterface.DeleteMulti(keys, func(internalErr error) {
		lme.Assign(i, internalErr)
		i++
	})
	err = lme.Get()
	if err == nil {
		err = extErr
	}
	return
}

func (d *datastoreImpl) Raw() RawInterface {
	return d.RawInterface
}
