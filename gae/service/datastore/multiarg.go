// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package datastore

import (
	"fmt"
	"reflect"

	"github.com/luci/luci-go/common/errors"
)

type multiArgType struct {
	getMGS  func(slot reflect.Value) MetaGetterSetter
	getPLS  func(slot reflect.Value) PropertyLoadSaver
	newElem func() reflect.Value
}

func (mat *multiArgType) getKey(aid, ns string, slot reflect.Value) (*Key, error) {
	return newKeyObjErr(aid, ns, mat.getMGS(slot))
}

func (mat *multiArgType) getPM(slot reflect.Value) (PropertyMap, error) {
	return mat.getPLS(slot).Save(true)
}

func (mat *multiArgType) getMetaPM(slot reflect.Value) PropertyMap {
	return mat.getMGS(slot).GetAllMeta()
}

func (mat *multiArgType) setPM(slot reflect.Value, pm PropertyMap) error {
	return mat.getPLS(slot).Load(pm)
}

func (mat *multiArgType) setKey(slot reflect.Value, k *Key) {
	PopulateKey(mat.getMGS(slot), k)
}

// parseArg checks that et is of type S, *S, I, P or *P, for some
// struct type S, for some interface type I, or some non-interface non-pointer
// type P such that P or *P implements PropertyLoadSaver.
//
// If et is a chan type that implements PropertyLoadSaver, new elements will be
// allocated with a buffer of 0.
//
// If keysOnly is true, a read-only key extraction multiArgType will be
// returned if et is a *Key.
func parseArg(et reflect.Type, keysOnly bool) *multiArgType {
	var mat multiArgType

	if keysOnly && et == typeOfKey {
		mat.getMGS = func(slot reflect.Value) MetaGetterSetter { return &keyExtractionMGS{key: slot.Interface().(*Key)} }
		return &mat
	}

	// If we do identify a structCodec for this type, retain it so we don't
	// resolve it multiple times.
	var codec *structCodec
	initCodec := func(t reflect.Type) *structCodec {
		if codec == nil {
			codec = getCodec(t)
		}
		return codec
	}

	// Fill in MetaGetterSetter functions.
	switch {
	case et.Implements(typeOfMetaGetterSetter):
		// MGS
		mat.getMGS = func(slot reflect.Value) MetaGetterSetter { return slot.Interface().(MetaGetterSetter) }

	case reflect.PtrTo(et).Implements(typeOfMetaGetterSetter):
		// *MGS
		mat.getMGS = func(slot reflect.Value) MetaGetterSetter { return slot.Addr().Interface().(MetaGetterSetter) }

	default:
		switch et.Kind() {
		case reflect.Interface:
			// I
			mat.getMGS = func(slot reflect.Value) MetaGetterSetter { return getMGS(slot.Elem().Interface()) }

		case reflect.Ptr:
			// *S
			if et.Elem().Kind() != reflect.Struct {
				// Not a struct pointer.
				return nil
			}

			initCodec(et.Elem())
			mat.getMGS = func(slot reflect.Value) MetaGetterSetter { return &structPLS{slot.Elem(), codec, nil} }

		case reflect.Struct:
			// S
			initCodec(et)
			mat.getMGS = func(slot reflect.Value) MetaGetterSetter { return &structPLS{slot, codec, nil} }

		default:
			// Don't know how to get MGS for this type.
			return nil
		}
	}

	// Fill in PropertyLoadSaver functions.
	switch {
	case et.Implements(typeOfPropertyLoadSaver):
		// PLS
		mat.getPLS = func(slot reflect.Value) PropertyLoadSaver { return slot.Interface().(PropertyLoadSaver) }

	case reflect.PtrTo(et).Implements(typeOfPropertyLoadSaver):
		// *PLS
		mat.getPLS = func(slot reflect.Value) PropertyLoadSaver { return slot.Addr().Interface().(PropertyLoadSaver) }

	default:
		switch et.Kind() {
		case reflect.Interface:
			// I
			mat.getPLS = func(slot reflect.Value) PropertyLoadSaver {
				obj := slot.Elem().Interface()
				if pls, ok := obj.(PropertyLoadSaver); ok {
					return pls
				}
				return GetPLS(obj)
			}

		case reflect.Ptr:
			// *S
			if et.Elem().Kind() != reflect.Struct {
				// Not a struct pointer.
				return nil
			}
			initCodec(et.Elem())
			mat.getPLS = func(slot reflect.Value) PropertyLoadSaver { return &structPLS{slot.Elem(), codec, nil} }

		case reflect.Struct:
			// S
			initCodec(et)
			mat.getPLS = func(slot reflect.Value) PropertyLoadSaver { return &structPLS{slot, codec, nil} }

		default:
			// Don't know how to get PLS for this type.
			return nil
		}
	}

	// Generate new element.
	//
	// If a map/chan type implements an interface, its pointer is also considered
	// to implement that interface.
	//
	// In this case, we have special pointer-to-map/chan logic in multiArgTypePLS.
	mat.newElem = func() reflect.Value {
		return reflect.New(et).Elem()
	}

	switch et.Kind() {
	case reflect.Map:
		mat.newElem = func() reflect.Value {
			return reflect.MakeMap(et)
		}

	case reflect.Chan:
		mat.newElem = func() reflect.Value {
			return reflect.MakeChan(et, 0)
		}

	case reflect.Ptr:
		elem := et.Elem()
		switch elem.Kind() {
		case reflect.Map:
			mat.newElem = func() reflect.Value {
				ptr := reflect.New(elem)
				ptr.Elem().Set(reflect.MakeMap(elem))
				return ptr
			}

		case reflect.Chan:
			mat.newElem = func() reflect.Value {
				ptr := reflect.New(elem)
				ptr.Elem().Set(reflect.MakeChan(elem, 0))
				return ptr
			}

		default:
			mat.newElem = func() reflect.Value { return reflect.New(et.Elem()) }
		}

	case reflect.Interface:
		mat.newElem = nil
	}

	return &mat
}

// mustParseMultiArg checks that v has type []S, []*S, []I, []P or []*P, for
// some struct type S, for some interface type I, or some non-interface
// non-pointer type P such that P or *P implements PropertyLoadSaver.
func mustParseMultiArg(et reflect.Type) *multiArgType {
	if et.Kind() != reflect.Slice {
		panic(fmt.Errorf("invalid argument type: expected slice, got %s", et))
	}
	return mustParseArg(et.Elem())
}

func mustParseArg(et reflect.Type) *multiArgType {
	if mat := parseArg(et, false); mat != nil {
		return mat
	}
	panic(fmt.Errorf("invalid argument type: %s is not a PLS or pointer-to-struct", et))
}

func newKeyObjErr(aid, ns string, mgs MetaGetterSetter) (*Key, error) {
	if key, _ := GetMetaDefault(mgs, "key", nil).(*Key); key != nil {
		return key, nil
	}

	// get kind
	kind := GetMetaDefault(mgs, "kind", "").(string)
	if kind == "" {
		return nil, errors.New("unable to extract $kind")
	}

	// get id - allow both to be default for default keys
	sid := GetMetaDefault(mgs, "id", "").(string)
	iid := GetMetaDefault(mgs, "id", 0).(int64)

	// get parent
	par, _ := GetMetaDefault(mgs, "parent", nil).(*Key)

	return NewKey(aid, ns, kind, sid, iid, par), nil
}

func isOKSingleType(t reflect.Type, keysOnly bool) error {
	switch {
	case t == nil:
		return errors.New("no type information")
	case t.Implements(typeOfPropertyLoadSaver):
		return nil
	case !keysOnly && t == typeOfKey:
		return errors.New("not user datatype")

	case t.Kind() != reflect.Ptr:
		return errors.New("not a pointer")
	case t.Elem().Kind() != reflect.Struct:
		return errors.New("does not point to a struct")

	default:
		return nil
	}
}

// keyExtractionMGS is a MetaGetterSetter that wraps a key and only implements
// GetMeta, and even then only retrieves the wrapped key meta value, "key".
type keyExtractionMGS struct {
	MetaGetterSetter

	key *Key
}

func (mgs *keyExtractionMGS) GetMeta(key string) (interface{}, bool) {
	if key == "key" {
		return mgs.key, true
	}
	return nil, false
}

type metaMultiArgElement struct {
	arg  reflect.Value
	mat  *multiArgType
	size int // size is -1 if this element is not a slice.
}

type metaMultiArg struct {
	elems    []metaMultiArgElement
	keysOnly bool

	count int // total number of elements, flattening slices
}

func makeMetaMultiArg(args []interface{}, keysOnly bool) (*metaMultiArg, error) {
	mma := metaMultiArg{
		elems:    make([]metaMultiArgElement, len(args)),
		keysOnly: keysOnly,
	}

	lme := errors.NewLazyMultiError(len(args))
	for i, arg := range args {
		if arg == nil {
			lme.Assign(i, errors.New("cannot use nil as single argument"))
			continue
		}

		v := reflect.ValueOf(arg)
		vt := v.Type()
		mma.elems[i].arg = v

		// Try and treat the argument as a single-value first. This allows slices
		// that implement PropertyLoadSaver to be properly treated as a single
		// element.
		var err error
		isSlice := false
		mat := parseArg(vt, keysOnly)
		if mat == nil {
			// If this is a slice, treat it as a slice of arg candidates.
			if v.Kind() == reflect.Slice {
				isSlice = true
				mat = parseArg(vt.Elem(), keysOnly)
			}
		} else {
			// Single types need to be able to be assigned to.
			err = isOKSingleType(vt, keysOnly)
		}
		if mat == nil {
			err = errors.New("not a PLS or pointer-to-struct")
		}
		if err != nil {
			lme.Assign(i, fmt.Errorf("invalid input type (%T): %s", arg, err))
			continue
		}

		mma.elems[i].mat = mat
		if isSlice {
			l := v.Len()
			mma.count += l
			mma.elems[i].size = l
		} else {
			mma.count++
			mma.elems[i].size = -1
		}
	}
	if err := lme.Get(); err != nil {
		return nil, err
	}

	return &mma, nil
}

func (mma *metaMultiArg) iterator(cb metaMultiArgIteratorCallback) *metaMultiArgIterator {
	return &metaMultiArgIterator{
		metaMultiArg: mma,
		cb:           cb,
	}
}

// getKeysPMs returns the
func (mma *metaMultiArg) getKeysPMs(aid, ns string, meta bool) ([]*Key, []PropertyMap, error) {
	var et errorTracker
	it := mma.iterator(et.init(mma))

	// Determine our flattened keys and property maps.
	retKey := make([]*Key, mma.count)
	var retPM []PropertyMap
	if !mma.keysOnly {
		retPM = make([]PropertyMap, mma.count)
	}

	for i := 0; i < mma.count; i++ {
		it.next(func(mat *multiArgType, slot reflect.Value) error {
			key, err := mat.getKey(aid, ns, slot)
			if err != nil {
				return err
			}
			retKey[i] = key

			if !mma.keysOnly {
				var pm PropertyMap
				if meta {
					pm = mat.getMetaPM(slot)
				} else {
					var err error
					if pm, err = mat.getPM(slot); err != nil {
						return err
					}
				}
				retPM[i] = pm
			}
			return nil
		})
	}
	return retKey, retPM, et.error()
}

type metaMultiArgIterator struct {
	*metaMultiArg

	cb metaMultiArgIteratorCallback

	index   int // flattened index
	elemIdx int // current index in slice
	slotIdx int // current index within elemIdx element (0 if single)
}

func (it *metaMultiArgIterator) next(fn func(*multiArgType, reflect.Value) error) {
	if it.remaining() <= 0 {
		panic("out of bounds")
	}

	// Advance to the next populated element/slot.
	elem := &it.elems[it.elemIdx]
	if it.index > 0 {
		for {
			it.slotIdx++
			if it.slotIdx >= elem.size {
				it.elemIdx++
				it.slotIdx = 0
				elem = &it.elems[it.elemIdx]
			}

			// We're done iterating, unless we're on a zero-sized slice element.
			if elem.size != 0 {
				break
			}
		}
	}

	// Get the current slot value.
	slot := elem.arg
	if elem.size >= 0 {
		// slot is a slice type, get its member.
		slot = slot.Index(it.slotIdx)
	}

	// Execute our callback.
	it.cb(it, fn(elem.mat, slot))

	// Advance our flattened index.
	it.index++
}

func (it *metaMultiArgIterator) remaining() int {
	return it.count - it.index
}

type metaMultiArgIteratorCallback func(*metaMultiArgIterator, error)

type errorTracker struct {
	elemErrors errors.MultiError
}

func (et *errorTracker) init(mma *metaMultiArg) metaMultiArgIteratorCallback {
	return et.trackError
}

func (et *errorTracker) trackError(it *metaMultiArgIterator, err error) {
	if err == nil {
		return
	}

	if et.elemErrors == nil {
		et.elemErrors = make(errors.MultiError, len(it.elems))
	}

	// If this is a single element, assign the error directly.
	elem := it.elems[it.elemIdx]
	if elem.size < 0 {
		et.elemErrors[it.elemIdx] = err
	} else {
		// This is a slice element. Use a slice-sized MultiError for its element
		// error slot, then add this error to the inner MultiError's slot index.
		serr, ok := et.elemErrors[it.elemIdx].(errors.MultiError)
		if !ok {
			serr = make(errors.MultiError, elem.size)
			et.elemErrors[it.elemIdx] = serr
		}
		serr[it.slotIdx] = err
	}
}

func (et *errorTracker) error() error {
	if err := et.elemErrors; err != nil {
		return err
	}
	return nil
}

type boolTracker struct {
	errorTracker

	res ExistsResult
}

func (bt *boolTracker) init(mma *metaMultiArg) metaMultiArgIteratorCallback {
	bt.errorTracker.init(mma)

	sizes := make([]int, len(mma.elems))
	for i, e := range mma.elems {
		if e.size < 0 {
			sizes[i] = 1
		} else {
			sizes[i] = e.size
		}
	}

	bt.res.init(sizes...)
	return bt.trackExistsResult
}

func (bt *boolTracker) trackExistsResult(it *metaMultiArgIterator, err error) {
	switch err {
	case nil:
		bt.res.set(it.elemIdx, it.slotIdx)

	case ErrNoSuchEntity:
		break

	default:
		// Pass through to track as MultiError.
		bt.errorTracker.trackError(it, err)
	}
}

func (bt *boolTracker) result() *ExistsResult {
	bt.res.updateSlices()
	return &bt.res
}
