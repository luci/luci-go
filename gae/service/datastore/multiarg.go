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

package datastore

import (
	"fmt"
	"reflect"
	"sort"
	"sync"

	"go.chromium.org/luci/common/errors"
)

type metaMultiArgConstraints int

const (
	// mmaReadWrite allows a metaMultiArg to operate on any type that can be
	// both read and written to.
	mmaReadWrite metaMultiArgConstraints = iota
	// mmaKeysOnly implies mmaReadWrite, with the further statement that the only
	// operation that will be performed against the arguments will be key
	// extraction.
	mmaKeysOnly = iota
	// mmaWriteKeys indicates that the caller is only going to write key
	// values. This enables the same inputs as mmaReadWrite, but also allows
	// []*Key.
	mmaWriteKeys = iota
)

func (c metaMultiArgConstraints) allowSingleKey() bool {
	return c == mmaKeysOnly
}

func (c metaMultiArgConstraints) keyOperationsOnly() bool {
	return c >= mmaKeysOnly
}

type multiArgType struct {
	getMGS  func(slot reflect.Value) MetaGetterSetter
	getPLS  func(slot reflect.Value) PropertyLoadSaver
	newElem func() reflect.Value
}

func (mat *multiArgType) getKey(kc KeyContext, slot reflect.Value) (*Key, error) {
	return newKeyObjErr(kc, mat.getMGS(slot))
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

func (mat *multiArgType) setKey(slot reflect.Value, k *Key) bool {
	return populateKeyMGS(mat.getMGS(slot), k)
}

// parseArg checks that et is of type S, *S, I, P or *P, for some
// struct type S, for some interface type I, or some non-interface non-pointer
// type P such that P or *P implements PropertyLoadSaver.
//
// If et is a chan type that implements PropertyLoadSaver, new elements will be
// allocated with a buffer of 0.
//
// If allowKey is true, et may additional be type *Key. Only MetaGetterSetter
// fields will be populated in the result (see keyMGS).
func parseArg(et reflect.Type, allowKeys bool) *multiArgType {
	var mat multiArgType

	if et == typeOfKey {
		if !allowKeys {
			return nil
		}

		mat.getMGS = func(slot reflect.Value) MetaGetterSetter { return &keyMGS{slot: slot} }
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
	return mustParseArg(et.Elem(), true)
}

func mustParseArg(et reflect.Type, sliceArg bool) *multiArgType {
	if mat := parseArg(et, false); mat != nil {
		return mat
	}
	panic(fmt.Errorf("invalid argument type: %s is not a PLS or pointer-to-struct", et))
}

func newKeyObjErr(kc KeyContext, mgs MetaGetterSetter) (*Key, error) {
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

	return kc.NewKey(kind, sid, iid, par), nil
}

func isOKSingleType(t reflect.Type, allowKey bool) error {
	switch {
	case t == nil:
		return errors.New("no type information")
	case t.Implements(typeOfPropertyLoadSaver):
		return nil
	case !allowKey && t == typeOfKey:
		return errors.New("not user datatype")

	case t.Kind() != reflect.Ptr:
		return errors.New("not a pointer")
	case t.Elem().Kind() != reflect.Struct:
		return errors.New("does not point to a struct")

	default:
		return nil
	}
}

// keyMGS is a MetaGetterSetter that wraps a single key value/slot. It only
// implements operations on the "key" key.
//
// GetMeta will be implemented, returning the *Key for the "key" meta.
//
// If slot is addressable, SetMeta will allow it to be set to the supplied
// Value.
type keyMGS struct {
	slot reflect.Value
}

func (mgs *keyMGS) GetAllMeta() PropertyMap {
	return PropertyMap{"$key": MkPropertyNI(mgs.slot.Interface())}
}

func (mgs *keyMGS) GetMeta(key string) (interface{}, bool) {
	if key != "key" {
		return nil, false
	}
	return mgs.slot.Interface(), true
}

func (mgs *keyMGS) SetMeta(key string, value interface{}) bool {
	if !(key == "key" && mgs.slot.CanAddr()) {
		return false
	}
	mgs.slot.Set(reflect.ValueOf(value))
	return true
}

// metaMultiArgIndex is a two-dimensional index into a metaMultiArg.
type metaMultiArgIndex struct {
	// elem is the index of the element in a metaMultiArg.
	elem int
	// slot is the index within the specified element. If the element is not a
	// slice, slot will always be 0.
	slot int
}

type metaMultiArgElement struct {
	arg reflect.Value
	mat *multiArgType

	// size is -1 if this element is not a slice.
	size int
	// offset is the offset of the first element in the flattened space.
	offset int
}

// slot returns the element at the specified slot. If idx is invalid, slot will
// panic.
//
// If arg is a single value, the only valid index is 0. if arg is a slice, idx
// will be the offset within that slice.
func (e *metaMultiArgElement) slot(idx int) reflect.Value {
	if e.size >= 0 {
		return e.arg.Index(idx)
	}

	if idx != 0 {
		panic(fmt.Errorf("invalid slot index %d for single element", idx))
	}
	return e.arg
}

// length returns the number of elements in this metaMultiArgElement.
//
// If it represents a slice, the number of elements will be the length of that
// slice. If it represents a single value, the length will be 1.
func (e *metaMultiArgElement) length() int {
	if e.size >= 0 {
		return e.size
	}
	return 1
}

type metaMultiArg struct {
	// elems is the set of metaMultiArgElement entries, each of which represents
	// either a single value or a slice of single values.
	elems    []metaMultiArgElement
	keysOnly bool

	// count is the total number of elements, flattening slices.
	count int
	// flat, if true, means that this metaMultiArg consists entirely of single
	// elements (no slices). This is used as a lookup optimization to avoid binary
	// index search.
	flat bool
}

// makeMetaMultiArg returns a metaMultiArg for the supplied args.
//
// If an arg is a slice, a slice metaMultiArg will be returned, and errors for
// that slice will be written into a positional MultiError if they occur.
//
// If keysOnly is true, the caller is instructing metaMultiArg to only extract
// the datastore *Key from args. *Key entries will be permitted, but the caller
// may not write to them (since keys are read-only structs).
func makeMetaMultiArg(args []interface{}, c metaMultiArgConstraints) (*metaMultiArg, error) {
	mma := metaMultiArg{
		elems:    make([]metaMultiArgElement, len(args)),
		keysOnly: c.keyOperationsOnly(),
		flat:     true,
	}

	lme := errors.NewLazyMultiError(len(args))
	for i, arg := range args {
		if arg == nil {
			lme.Assign(i, errors.New("cannot use nil as single argument"))
			continue
		}

		v := reflect.ValueOf(arg)
		vt := v.Type()

		// Try and treat the argument as a single-value first. This allows slices
		// that implement PropertyLoadSaver to be properly treated as a single
		// element.
		var (
			mat     *multiArgType
			err     error
			isSlice = false
		)
		if mat = parseArg(vt, c.allowSingleKey()); mat == nil {
			// If this is a slice, treat it as a slice of arg candidates.
			//
			// If only keys are being read/written, we allow a []*Key to be accepted
			// here, since slices are addressable (write).
			if v.Kind() == reflect.Slice {
				isSlice = true
				mma.flat = false
				mat = parseArg(vt.Elem(), c.keyOperationsOnly())
			}
		} else {
			// Single types need to be able to be assigned to.
			//
			// We only allow *Key here when the keys cannot be written to, since *Key
			// should not be modified in-place, as they are immutable.
			err = isOKSingleType(vt, c.allowSingleKey())
		}
		if mat == nil && err == nil {
			err = errors.New("not a PLS, pointer-to-struct, or slice thereof")
		}
		if err != nil {
			lme.Assign(i, fmt.Errorf("invalid input type (%T): %s", arg, err))
			continue
		}

		elem := &mma.elems[i]
		*elem = metaMultiArgElement{
			arg:    v,
			mat:    mat,
			offset: mma.count,
		}
		if isSlice {
			l := v.Len()
			mma.count += l
			elem.size = l
		} else {
			mma.count++
			elem.size = -1
		}
	}
	if err := lme.Get(); err != nil {
		return nil, err
	}

	return &mma, nil
}

// get returns the element type and value at flattened index idx.
func (mma *metaMultiArg) index(idx int) (mmaIdx metaMultiArgIndex) {
	if mma.flat {
		mmaIdx.elem = idx
		return
	}

	mmaIdx.elem = sort.Search(len(mma.elems), func(i int) bool { return mma.elems[i].offset > idx }) - 1

	// Get the current slot value.
	mmaIdx.slot = idx - mma.elems[mmaIdx.elem].offset
	return
}

func (mma *metaMultiArg) get(idx metaMultiArgIndex) (*multiArgType, reflect.Value) {
	// Get the current slot value.
	elem := &mma.elems[idx.elem]
	slot := elem.arg
	if elem.size >= 0 {
		// slot is a slice type, get its member.
		slot = slot.Index(idx.slot)
	}

	return elem.mat, slot
}

// getKeysPMs returns the keys and PropertyMap for the supplied argument items.
func (mma *metaMultiArg) getKeysPMs(kc KeyContext, meta bool) ([]*Key, []PropertyMap, error) {
	et := newErrorTracker(mma)

	// Determine our flattened keys and property maps.
	retKey := make([]*Key, mma.count)
	var retPM []PropertyMap
	if !mma.keysOnly {
		retPM = make([]PropertyMap, mma.count)
	}

	var index metaMultiArgIndex
	for i := 0; i < mma.count; i++ {
		// If we're past the end of the element, move onto the next.
		for index.slot >= mma.elems[index.elem].length() {
			index.elem++
			index.slot = 0
		}

		mat, slot := mma.get(index)
		key, err := mat.getKey(kc, slot)
		if err != nil {
			et.trackError(index, err)
			continue
		}
		retKey[i] = key

		if !mma.keysOnly {
			var pm PropertyMap
			if meta {
				pm = mat.getMetaPM(slot)
			} else {
				var err error
				if pm, err = mat.getPM(slot); err != nil {
					et.trackError(index, err)
					continue
				}
			}
			retPM[i] = pm
		}

		index.slot++
	}
	return retKey, retPM, et.error()
}

type errorTracker struct {
	sync.Mutex

	elemErrors errors.MultiError
	mma        *metaMultiArg
}

func newErrorTracker(mma *metaMultiArg) *errorTracker {
	return &errorTracker{
		mma: mma,
	}
}

func (et *errorTracker) trackError(index metaMultiArgIndex, err error) {
	if err == nil {
		return
	}

	et.Lock()
	defer et.Unlock()
	et.trackErrorLocked(index, err)
}

func (et *errorTracker) trackErrorLocked(index metaMultiArgIndex, err error) {
	if err == nil {
		return
	}

	if et.elemErrors == nil {
		et.elemErrors = make(errors.MultiError, len(et.mma.elems))
	}

	// If this is a single element, assign the error directly.
	elem := &et.mma.elems[index.elem]
	if elem.size < 0 {
		et.elemErrors[index.elem] = err
	} else {
		// This is a slice element. Use a slice-sized MultiError for its element
		// error slot, then add this error to the inner MultiError's slot index.
		serr, ok := et.elemErrors[index.elem].(errors.MultiError)
		if !ok {
			serr = make(errors.MultiError, elem.size)
			et.elemErrors[index.elem] = serr
		}
		serr[index.slot] = err
	}
}

func (et *errorTracker) error() error {
	if et.elemErrors != nil {
		return et.elemErrors
	}
	return nil
}

type boolTracker struct {
	*errorTracker

	res ExistsResult
}

func newBoolTracker(mma *metaMultiArg) *boolTracker {
	bt := boolTracker{
		errorTracker: newErrorTracker(mma),
	}

	sizes := make([]int, len(mma.elems))
	for i, e := range mma.elems {
		if e.size < 0 {
			sizes[i] = 1
		} else {
			sizes[i] = e.size
		}
	}
	bt.res.init(sizes...)
	return &bt
}

func (bt *boolTracker) trackExistsResult(index metaMultiArgIndex, err error) {
	bt.Lock()
	defer bt.Unlock()

	switch err {
	case nil:
		bt.res.set(index.elem, index.slot)

	case ErrNoSuchEntity:
		break

	default:
		// Pass through to track as MultiError.
		bt.trackErrorLocked(index, err)
	}
}

func (bt *boolTracker) result() *ExistsResult {
	bt.res.updateSlices()
	return &bt.res
}
