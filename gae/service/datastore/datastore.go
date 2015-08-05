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

func (d *datastoreImpl) Run(q Query, proto interface{}, cb RunCB) error {
	if _, ok := proto.(*Key); ok {
		return d.RawInterface.Run(q.KeysOnly(), func(k Key, _ PropertyMap, gc func() (Cursor, error)) bool {
			return cb(k, gc)
		})
	}

	mat := parseArg(reflect.TypeOf(proto))
	if !mat.valid || mat.newElem == nil {
		return fmt.Errorf("invalid Run proto type: %T", proto)
	}

	innerErr := error(nil)
	err := d.RawInterface.Run(q, func(k Key, pm PropertyMap, gc func() (Cursor, error)) bool {
		itm := mat.newElem()
		if innerErr = mat.setPM(itm, pm); innerErr != nil {
			return false
		}
		mat.setKey(itm, k)
		return cb(itm.Interface(), gc)
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
		return d.RawInterface.Run(q.KeysOnly(), func(k Key, _ PropertyMap, _ func() (Cursor, error)) bool {
			*keys = append(*keys, k)
			return true
		})
	}

	slice := v.Elem()
	mat := parseMultiArg(slice.Type())
	if !mat.valid || mat.newElem == nil {
		return fmt.Errorf("invalid GetAll input type: %T", dst)
	}

	lme := errors.LazyMultiError{Size: slice.Len()}
	i := 0
	err := d.RawInterface.Run(q, func(k Key, pm PropertyMap, _ func() (Cursor, error)) bool {
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

	lme := errors.LazyMultiError{Size: len(keys)}
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

	lme := errors.LazyMultiError{Size: len(keys)}
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
	lme := errors.LazyMultiError{Size: len(keys)}
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
