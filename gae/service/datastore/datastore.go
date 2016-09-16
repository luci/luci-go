// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package datastore

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	"github.com/luci/luci-go/common/errors"

	"gopkg.in/yaml.v2"
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
	return newKeyObjErr(d.aid, d.ns, getMGS(src))
}

func (d *datastoreImpl) MakeKey(elems ...interface{}) *Key {
	return MakeKey(d.aid, d.ns, elems...)
}

func (d *datastoreImpl) NewKey(kind, stringID string, intID int64, parent *Key) *Key {
	return NewKey(d.aid, d.ns, kind, stringID, intID, parent)
}

func (d *datastoreImpl) NewIncompleteKeys(count int, kind string, parent *Key) (keys []*Key) {
	if count > 0 {
		keys = make([]*Key, count)
		for i := range keys {
			keys[i] = d.NewKey(kind, "", 0, parent)
		}
	}
	return
}

func (d *datastoreImpl) NewKeyToks(toks []KeyTok) *Key {
	return NewKeyToks(d.aid, d.ns, toks)
}

// PopulateKey loads key into obj.
//
// obj is any object that Interface.Get is able to accept.
//
// Upon successful application, this method will return true. If the key could
// not be applied to the object, this method will return false. It will panic if
// obj is an invalid datastore model.
//
// This method will panic if obj is an invalid datastore model. If the key could
// not be applied to the object, nothing will happen.
func PopulateKey(obj interface{}, key *Key) bool {
	return populateKeyMGS(getMGS(obj), key)
}

func populateKeyMGS(mgs MetaGetterSetter, key *Key) bool {
	if mgs.SetMeta("key", key) {
		return true
	}

	lst := key.LastTok()
	if lst.StringID != "" {
		if !mgs.SetMeta("id", lst.StringID) {
			return false
		}
	} else {
		if !mgs.SetMeta("id", lst.IntID) {
			return false
		}
	}

	mgs.SetMeta("kind", lst.Kind)
	mgs.SetMeta("parent", key.Parent())
	return true
}

func checkMultiSliceType(v interface{}) error {
	if reflect.TypeOf(v).Kind() == reflect.Slice {
		return nil
	}
	return fmt.Errorf("argument must be a slice, not %T", v)

}

func runParseCallback(cbIface interface{}) (isKey, hasErr, hasCursorCB bool, mat *multiArgType) {
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
		mat = mustParseArg(firstArg, false)
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

func (d *datastoreImpl) AllocateIDs(ent ...interface{}) error {
	if len(ent) == 0 {
		return nil
	}

	mma, err := makeMetaMultiArg(ent, mmaWriteKeys)
	if err != nil {
		panic(err)
	}

	keys, _, err := mma.getKeysPMs(d.aid, d.ns, false)
	if err != nil {
		if len(ent) == 1 {
			// Single-argument Exists will return a single error.
			err = errors.SingleError(err)
		}
		return err
	}
	if len(keys) == 0 {
		return nil
	}

	// Convert each key to be partial valid, assigning an integer ID of 0. Confirm
	// that each object can be populated with such a key.
	for i, key := range keys {
		keys[i] = key.Incomplete()
	}

	var et errorTracker
	it := mma.iterator(et.init(mma))
	err = filterStop(d.RawInterface.AllocateIDs(keys, func(key *Key, err error) error {
		it.next(func(mat *multiArgType, v reflect.Value) error {
			if err != nil {
				return err
			}

			if !mat.setKey(v, key) {
				return ErrInvalidKey
			}
			return nil
		})

		return nil
	}))
	if err == nil {
		err = et.error()
	}

	if err != nil && len(ent) == 1 {
		// Single-argument Exists will return a single error.
		err = errors.SingleError(err)
	}
	return err
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
		err = d.RawInterface.Run(fq, func(k *Key, _ PropertyMap, gc CursorCB) error {
			return cb(reflect.ValueOf(k), gc)
		})
	} else {
		err = d.RawInterface.Run(fq, func(k *Key, pm PropertyMap, gc CursorCB) error {
			itm := mat.newElem()
			if err := mat.setPM(itm, pm); err != nil {
				return err
			}
			mat.setKey(itm, k)
			return cb(itm, gc)
		})
	}
	return filterStop(err)
}

func (d *datastoreImpl) Count(q *Query) (int64, error) {
	fq, err := q.Finalize()
	if err != nil {
		return 0, err
	}
	v, err := d.RawInterface.Count(fq)
	return v, filterStop(err)
}

func (d *datastoreImpl) GetAll(q *Query, dst interface{}) error {
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Ptr {
		panic(fmt.Errorf("invalid GetAll dst: must have a ptr-to-slice: %T", dst))
	}
	if !v.IsValid() || v.IsNil() {
		panic(errors.New("invalid GetAll dst: <nil>"))
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
	mat := mustParseMultiArg(slice.Type())
	if mat.newElem == nil {
		panic(fmt.Errorf("invalid GetAll dst (non-concrete element type): %T", dst))
	}

	errs := map[int]error{}
	i := 0
	err = filterStop(d.RawInterface.Run(fq, func(k *Key, pm PropertyMap, _ CursorCB) error {
		slice.Set(reflect.Append(slice, mat.newElem()))
		itm := slice.Index(i)
		mat.setKey(itm, k)
		err := mat.setPM(itm, pm)
		if err != nil {
			errs[i] = err
		}
		i++
		return nil
	}))
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

func (d *datastoreImpl) Exists(ent ...interface{}) (*ExistsResult, error) {
	if len(ent) == 0 {
		return nil, nil
	}

	mma, err := makeMetaMultiArg(ent, mmaKeysOnly)
	if err != nil {
		panic(err)
	}

	keys, _, err := mma.getKeysPMs(d.aid, d.ns, false)
	if err != nil {
		if len(ent) == 1 {
			// Single-argument Exists will return a single error.
			err = errors.SingleError(err)
		}
		return nil, err
	}
	if len(keys) == 0 {
		return nil, nil
	}

	var bt boolTracker
	it := mma.iterator(bt.init(mma))
	err = filterStop(d.RawInterface.GetMulti(keys, nil, func(_ PropertyMap, err error) error {
		it.next(func(*multiArgType, reflect.Value) error {
			return err
		})
		return nil
	}))
	if err == nil {
		err = bt.error()
	}

	if err != nil && len(ent) == 1 {
		// Single-argument Exists will return a single error.
		err = errors.SingleError(err)
	}
	return bt.result(), err
}

func (d *datastoreImpl) ExistsMulti(keys []*Key) (BoolList, error) {
	v, err := d.Exists(keys)
	if err != nil {
		return nil, err
	}
	return v.List(0), nil
}

func (d *datastoreImpl) Get(dst ...interface{}) (err error) {
	if len(dst) == 0 {
		return nil
	}

	mma, err := makeMetaMultiArg(dst, mmaReadWrite)
	if err != nil {
		panic(err)
	}

	keys, pms, err := mma.getKeysPMs(d.aid, d.ns, true)
	if err != nil {
		if len(dst) == 1 {
			// Single-argument Get will return a single error.
			err = errors.SingleError(err)
		}
		return err
	}
	if len(keys) == 0 {
		return nil
	}

	var et errorTracker
	it := mma.iterator(et.init(mma))
	meta := NewMultiMetaGetter(pms)
	err = filterStop(d.RawInterface.GetMulti(keys, meta, func(pm PropertyMap, err error) error {
		it.next(func(mat *multiArgType, slot reflect.Value) error {
			if err != nil {
				return err
			}
			return mat.setPM(slot, pm)
		})
		return nil
	}))

	if err == nil {
		err = et.error()
	}

	if err != nil && len(dst) == 1 {
		// Single-argument Get will return a single error.
		err = errors.SingleError(err)
	}
	return err
}

func (d *datastoreImpl) GetMulti(dst interface{}) error {
	if err := checkMultiSliceType(dst); err != nil {
		panic(err)
	}
	return d.Get(dst)
}

func (d *datastoreImpl) Put(src ...interface{}) (err error) {
	if len(src) == 0 {
		return nil
	}

	mma, err := makeMetaMultiArg(src, mmaReadWrite)
	if err != nil {
		panic(err)
	}

	keys, vals, err := mma.getKeysPMs(d.aid, d.ns, false)
	if err != nil {
		if len(src) == 1 {
			// Single-argument Put will return a single error.
			err = errors.SingleError(err)
		}
		return err
	}
	if len(keys) == 0 {
		return nil
	}

	i := 0
	var et errorTracker
	it := mma.iterator(et.init(mma))
	err = filterStop(d.RawInterface.PutMulti(keys, vals, func(key *Key, err error) error {
		it.next(func(mat *multiArgType, slot reflect.Value) error {
			if err != nil {
				return err
			}
			if key != keys[i] {
				mat.setKey(slot, key)
			}
			return nil
		})

		i++
		return nil
	}))

	if err == nil {
		err = et.error()
	}

	if err != nil && len(src) == 1 {
		// Single-argument Put will return a single error.
		err = errors.SingleError(err)
	}
	return err
}

func (d *datastoreImpl) PutMulti(src interface{}) error {
	if err := checkMultiSliceType(src); err != nil {
		panic(err)
	}
	return d.Put(src)
}

func (d *datastoreImpl) Delete(ent ...interface{}) error {
	if len(ent) == 0 {
		return nil
	}

	mma, err := makeMetaMultiArg(ent, mmaKeysOnly)
	if err != nil {
		panic(err)
	}

	keys, _, err := mma.getKeysPMs(d.aid, d.ns, false)
	if err != nil {
		if len(ent) == 1 {
			// Single-argument Delete will return a single error.
			err = errors.SingleError(err)
		}
		return err
	}
	if len(keys) == 0 {
		return nil
	}

	var et errorTracker
	it := mma.iterator(et.init(mma))
	err = filterStop(d.RawInterface.DeleteMulti(keys, func(err error) error {
		it.next(func(*multiArgType, reflect.Value) error {
			return err
		})

		return nil
	}))
	if err == nil {
		err = et.error()
	}

	if err != nil && len(ent) == 1 {
		// Single-argument Delete will return a single error.
		err = errors.SingleError(err)
	}
	return err
}

func (d *datastoreImpl) DeleteMulti(keys []*Key) error {
	return d.Delete(keys)
}

func (d *datastoreImpl) Raw() RawInterface {
	return d.RawInterface
}

// ParseIndexYAML parses the contents of a index YAML file into a list of
// IndexDefinitions.
func ParseIndexYAML(content io.Reader) ([]*IndexDefinition, error) {
	serialized, err := ioutil.ReadAll(content)
	if err != nil {
		return nil, err
	}

	var m map[string][]*IndexDefinition
	if err := yaml.Unmarshal(serialized, &m); err != nil {
		return nil, err
	}

	if _, ok := m["indexes"]; !ok {
		return nil, fmt.Errorf("datastore: missing key `indexes`: %v", m)
	}
	return m["indexes"], nil
}

// getCallingTestFilePath looks up the call stack until the specified
// maxStackDepth and returns the absolute path of the first source filename
// ending with `_test.go`. If no test file is found, getCallingTestFilePath
// returns a non-nil error.
func getCallingTestFilePath(maxStackDepth int) (string, error) {
	pcs := make([]uintptr, maxStackDepth)

	for _, pc := range pcs[:runtime.Callers(0, pcs)] {
		path, _ := runtime.FuncForPC(pc - 1).FileLine(pc - 1)
		if filename := filepath.Base(path); strings.HasSuffix(filename, "_test.go") {
			return path, nil
		}
	}

	return "", fmt.Errorf("datastore: failed to determine source file name")
}

// FindAndParseIndexYAML walks up from the directory specified by path until it
// finds a `index.yaml` or `index.yml` file. If an index YAML file
// is found, it opens and parses the file, and returns all the indexes found.
// If path is a relative path, it is converted into an absolute path
// relative to the calling test file. To determine the path of the calling test
// file, FindAndParseIndexYAML walks upto a maximum of 100 call stack frames
// looking for a file ending with `_test.go`.
//
// FindAndParseIndexYAML returns a non-nil error if the root of the drive is
// reached without finding an index YAML file, if there was
// an error reading the found index YAML file, or if the calling test file could
// not be located in the case of a relative path argument.
func FindAndParseIndexYAML(path string) ([]*IndexDefinition, error) {
	var currentDir string

	if filepath.IsAbs(path) {
		currentDir = path
	} else {
		testPath, err := getCallingTestFilePath(100)
		if err != nil {
			return nil, err
		}
		currentDir = filepath.Join(filepath.Dir(testPath), path)
	}

	isRoot := func(dir string) bool {
		parentDir := filepath.Dir(dir)
		return os.IsPathSeparator(dir[len(dir)-1]) && os.IsPathSeparator(parentDir[len(parentDir)-1])
	}

	for {
		for _, filename := range []string{"index.yml", "index.yaml"} {
			file, err := os.Open(filepath.Join(currentDir, filename))
			if err == nil {
				defer file.Close()
				return ParseIndexYAML(file)
			}
		}

		if isRoot(currentDir) {
			return nil, fmt.Errorf("datastore: failed to find index YAML file")
		}

		currentDir = filepath.Dir(currentDir)
	}
}

func filterStop(err error) error {
	if err == Stop {
		err = nil
	}
	return err
}
