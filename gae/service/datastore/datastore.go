// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package datastore

import (
	"fmt"
	"reflect"

	"github.com/luci/luci-go/common/errors"
)

type datastoreImpl struct {
	RawInterface

	aid string
	ns  string
}

var _ Interface = (*datastoreImpl)(nil)

func (d *datastoreImpl) KeyForObj(src interface{}) *Key {
	ret, err := d.KeyForObjErr(src)
	if err != nil {
		panic(err)
	}
	return ret
}

func (d *datastoreImpl) KeyForObjErr(src interface{}) (*Key, error) {
	return newKeyObjErr(d.aid, d.ns, src)
}

func (d *datastoreImpl) MakeKey(elems ...interface{}) *Key {
	return MakeKey(d.aid, d.ns, elems...)
}

func (d *datastoreImpl) NewKey(kind, stringID string, intID int64, parent *Key) *Key {
	return NewKey(d.aid, d.ns, kind, stringID, intID, parent)
}

func (d *datastoreImpl) NewKeyToks(toks []KeyTok) *Key {
	return NewKeyToks(d.aid, d.ns, toks)
}

func runParseCallback(cbIface interface{}) (isKey, hasErr, hasCursorCB bool, mat multiArgType) {
	badSig := func() {
		panic(fmt.Errorf(
			"cb does not match the required callback signature: `%T` != `func(TYPE, [CursorCB]) [error]`",
			cbIface))
	}

	if cbIface == nil {
		badSig()
	}

	// TODO(riannucci): Profile and determine if any of this is causing a real
	// slowdown. Could potentially cache reflection stuff by cbTyp?
	cbTyp := reflect.TypeOf(cbIface)

	if cbTyp.Kind() != reflect.Func {
		badSig()
	}

	numIn := cbTyp.NumIn()
	if numIn != 1 && numIn != 2 {
		badSig()
	}

	firstArg := cbTyp.In(0)
	if firstArg == typeOfKey {
		isKey = true
	} else {
		mat = parseArg(firstArg, false)
		if mat.newElem == nil {
			badSig()
		}
	}

	hasCursorCB = numIn == 2
	if hasCursorCB && cbTyp.In(1) != typeOfCursorCB {
		badSig()
	}

	if cbTyp.NumOut() > 1 {
		badSig()
	} else if cbTyp.NumOut() == 1 && cbTyp.Out(0) != typeOfError {
		badSig()
	}
	hasErr = cbTyp.NumOut() == 1

	return
}

func (d *datastoreImpl) Run(q *Query, cbIface interface{}) error {
	isKey, hasErr, hasCursorCB, mat := runParseCallback(cbIface)

	if isKey {
		q = q.KeysOnly(true)
	}
	fq, err := q.Finalize()
	if err != nil {
		return err
	}

	cbVal := reflect.ValueOf(cbIface)
	var cb func(reflect.Value, CursorCB) error
	switch {
	case hasErr && hasCursorCB:
		cb = func(v reflect.Value, cb CursorCB) error {
			err := cbVal.Call([]reflect.Value{v, reflect.ValueOf(cb)})[0].Interface()
			if err != nil {
				return err.(error)
			}
			return nil
		}

	case hasErr && !hasCursorCB:
		cb = func(v reflect.Value, _ CursorCB) error {
			err := cbVal.Call([]reflect.Value{v})[0].Interface()
			if err != nil {
				return err.(error)
			}
			return nil
		}

	case !hasErr && hasCursorCB:
		cb = func(v reflect.Value, cb CursorCB) error {
			cbVal.Call([]reflect.Value{v, reflect.ValueOf(cb)})
			return nil
		}

	case !hasErr && !hasCursorCB:
		cb = func(v reflect.Value, _ CursorCB) error {
			cbVal.Call([]reflect.Value{v})
			return nil
		}
	}

	if isKey {
		return d.RawInterface.Run(fq, func(k *Key, _ PropertyMap, gc CursorCB) error {
			return cb(reflect.ValueOf(k), gc)
		})
	}

	return d.RawInterface.Run(fq, func(k *Key, pm PropertyMap, gc CursorCB) error {
		itm := mat.newElem()
		if err := mat.setPM(itm, pm); err != nil {
			return err
		}
		mat.setKey(itm, k)
		return cb(itm, gc)
	})
}

func (d *datastoreImpl) Count(q *Query) (int64, error) {
	fq, err := q.Finalize()
	if err != nil {
		return 0, err
	}
	return d.RawInterface.Count(fq)
}

func (d *datastoreImpl) GetAll(q *Query, dst interface{}) error {
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Ptr {
		return fmt.Errorf("invalid GetAll dst: must have a ptr-to-slice: %T", dst)
	}
	if !v.IsValid() || v.IsNil() {
		return errors.New("invalid GetAll dst: <nil>")
	}

	if keys, ok := dst.(*[]*Key); ok {
		fq, err := q.KeysOnly(true).Finalize()
		if err != nil {
			return err
		}

		return d.RawInterface.Run(fq, func(k *Key, _ PropertyMap, _ CursorCB) error {
			*keys = append(*keys, k)
			return nil
		})
	}
	fq, err := q.Finalize()
	if err != nil {
		return err
	}

	slice := v.Elem()
	mat := parseMultiArg(slice.Type())
	if mat.newElem == nil {
		return fmt.Errorf("invalid GetAll input type: %T", dst)
	}

	errs := map[int]error{}
	i := 0
	err = d.RawInterface.Run(fq, func(k *Key, pm PropertyMap, _ CursorCB) error {
		slice.Set(reflect.Append(slice, mat.newElem()))
		itm := slice.Index(i)
		mat.setKey(itm, k)
		err := mat.setPM(itm, pm)
		if err != nil {
			errs[i] = err
		}
		i++
		return nil
	})
	if err == nil {
		if len(errs) > 0 {
			me := make(errors.MultiError, slice.Len())
			for i, e := range errs {
				me[i] = e
			}
			err = me
		}
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

func (d *datastoreImpl) ExistsMulti(keys []*Key) ([]bool, error) {
	lme := errors.NewLazyMultiError(len(keys))
	ret := make([]bool, len(keys))
	i := 0
	err := d.RawInterface.GetMulti(keys, nil, func(_ PropertyMap, err error) error {
		if err == nil {
			ret[i] = true
		} else if err != ErrNoSuchEntity {
			lme.Assign(i, err)
		}
		i++
		return nil
	})
	if err != nil {
		return ret, err
	}
	return ret, lme.Get()
}

func (d *datastoreImpl) Exists(k *Key) (bool, error) {
	ret, err := d.ExistsMulti([]*Key{k})
	return ret[0], errors.SingleError(err)
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

func (d *datastoreImpl) Delete(key *Key) (err error) {
	return errors.SingleError(d.DeleteMulti([]*Key{key}))
}

func (d *datastoreImpl) GetMulti(dst interface{}) error {
	slice := reflect.ValueOf(dst)
	mat := parseMultiArg(slice.Type())

	keys, pms, err := mat.GetKeysPMs(d.aid, d.ns, slice, true)
	if err != nil {
		return err
	}

	lme := errors.NewLazyMultiError(len(keys))
	i := 0
	meta := NewMultiMetaGetter(pms)
	err = d.RawInterface.GetMulti(keys, meta, func(pm PropertyMap, err error) error {
		if !lme.Assign(i, err) {
			lme.Assign(i, mat.setPM(slice.Index(i), pm))
		}
		i++
		return nil
	})

	if err == nil {
		err = lme.Get()
	}
	return err
}

func (d *datastoreImpl) PutMulti(src interface{}) error {
	slice := reflect.ValueOf(src)
	mat := parseMultiArg(slice.Type())

	keys, vals, err := mat.GetKeysPMs(d.aid, d.ns, slice, false)
	if err != nil {
		return err
	}

	lme := errors.NewLazyMultiError(len(keys))
	i := 0
	err = d.RawInterface.PutMulti(keys, vals, func(key *Key, err error) error {
		if key != keys[i] {
			mat.setKey(slice.Index(i), key)
		}
		lme.Assign(i, err)
		i++
		return nil
	})

	if err == nil {
		err = lme.Get()
	}
	return err
}

func (d *datastoreImpl) DeleteMulti(keys []*Key) (err error) {
	lme := errors.NewLazyMultiError(len(keys))
	i := 0
	extErr := d.RawInterface.DeleteMulti(keys, func(internalErr error) error {
		lme.Assign(i, internalErr)
		i++
		return nil
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
