// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// HEAVILY adapted from github.com/golang/appengine/datastore

package helper

import (
	"errors"
	"fmt"
	"infra/gae/libs/gae"
	"reflect"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Entities with more than this many indexed properties will not be saved.
const maxIndexedProperties = 20000

var (
	typeOfDSKey               = reflect.TypeOf((*gae.DSKey)(nil)).Elem()
	typeOfDSPropertyConverter = reflect.TypeOf((*gae.DSPropertyConverter)(nil)).Elem()
	typeOfGeoPoint            = reflect.TypeOf(gae.DSGeoPoint{})
	typeOfTime                = reflect.TypeOf(time.Time{})

	valueOfnilDSKey = reflect.Zero(typeOfDSKey)
)

type structTag struct {
	name           string
	noIndex        bool
	isSlice        bool
	substructCodec *structCodec
	convert        bool
	specialVal     string
}

type structCodec struct {
	bySpecial map[string]int
	byName    map[string]int
	byIndex   []structTag
	hasSlice  bool
	problem   error
}

type structPLS struct {
	o reflect.Value
	c *structCodec
}

var _ gae.DSStructPLS = (*structPLS)(nil)

// typeMismatchReason returns a string explaining why the property p could not
// be stored in an entity field of type v.Type().
func typeMismatchReason(val interface{}, v reflect.Value) string {
	entityType := reflect.TypeOf(val)
	return fmt.Sprintf("type mismatch: %s versus %v", entityType, v.Type())
}

func (p *structPLS) Load(propMap gae.DSPropertyMap) (convFailures []string, fatal error) {
	if fatal = p.Problem(); fatal != nil {
		return
	}

	t := p.o.Type()
	for name, props := range propMap {
		multiple := len(props) > 1
		for i, prop := range props {
			if reason := loadInner(p.c, p.o, i, name, prop, multiple); reason != "" {
				convFailures = append(convFailures, fmt.Sprintf(
					"cannot load field %q into a %q: %s", name, t, reason))
			}
		}
	}
	return
}

func loadInner(codec *structCodec, structValue reflect.Value, index int, name string, p gae.DSProperty, requireSlice bool) string {
	var v reflect.Value
	// Traverse a struct's struct-typed fields.
	for {
		fieldIndex, ok := codec.byName[name]
		if !ok {
			return "no such struct field"
		}
		v = structValue.Field(fieldIndex)

		st := codec.byIndex[fieldIndex]
		if st.substructCodec == nil {
			break
		}

		if v.Kind() == reflect.Slice {
			for v.Len() <= index {
				v.Set(reflect.Append(v, reflect.New(v.Type().Elem()).Elem()))
			}
			structValue = v.Index(index)
			requireSlice = false
		} else {
			structValue = v
		}
		// Strip the "I." from "I.X".
		name = name[len(st.name):]
		codec = st.substructCodec
	}

	doConversion := func(v reflect.Value) (string, bool) {
		a := v.Addr()
		if conv, ok := a.Interface().(gae.DSPropertyConverter); ok {
			err := conv.FromDSProperty(p)
			if err != nil {
				return err.Error(), true
			}
			return "", true
		}
		return "", false
	}

	if ret, ok := doConversion(v); ok {
		return ret
	}

	var slice reflect.Value
	if v.Kind() == reflect.Slice && v.Type().Elem().Kind() != reflect.Uint8 {
		slice = v
		v = reflect.New(v.Type().Elem()).Elem()
	} else if requireSlice {
		return "multiple-valued property requires a slice field type"
	}

	pVal := p.Value()

	if ret, ok := doConversion(v); ok {
		if ret != "" {
			return ret
		}
	} else {
		knd := v.Kind()
		if v.Type().Implements(typeOfDSKey) {
			knd = reflect.Interface
		}
		switch knd {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			x, ok := pVal.(int64)
			if !ok && pVal != nil {
				return typeMismatchReason(pVal, v)
			}
			if v.OverflowInt(x) {
				return fmt.Sprintf("value %v overflows struct field of type %v", x, v.Type())
			}
			v.SetInt(x)
		case reflect.Bool:
			x, ok := pVal.(bool)
			if !ok && pVal != nil {
				return typeMismatchReason(pVal, v)
			}
			v.SetBool(x)
		case reflect.String:
			switch x := pVal.(type) {
			case gae.BSKey:
				v.SetString(string(x))
			case string:
				v.SetString(x)
			default:
				if pVal != nil {
					return typeMismatchReason(pVal, v)
				}
			}
		case reflect.Float32, reflect.Float64:
			x, ok := pVal.(float64)
			if !ok && pVal != nil {
				return typeMismatchReason(pVal, v)
			}
			if v.OverflowFloat(x) {
				return fmt.Sprintf("value %v overflows struct field of type %v", x, v.Type())
			}
			v.SetFloat(x)
		case reflect.Interface:
			x, ok := pVal.(gae.DSKey)
			if !ok && pVal != nil {
				return typeMismatchReason(pVal, v)
			}
			if x != nil {
				v.Set(reflect.ValueOf(x))
			}
		case reflect.Struct:
			switch v.Type() {
			case typeOfTime:
				x, ok := pVal.(time.Time)
				if !ok && pVal != nil {
					return typeMismatchReason(pVal, v)
				}
				v.Set(reflect.ValueOf(x))
			case typeOfGeoPoint:
				x, ok := pVal.(gae.DSGeoPoint)
				if !ok && pVal != nil {
					return typeMismatchReason(pVal, v)
				}
				v.Set(reflect.ValueOf(x))
			default:
				panic(fmt.Errorf("helper: impossible: %s", typeMismatchReason(pVal, v)))
			}
		case reflect.Slice:
			switch x := pVal.(type) {
			case []byte:
				v.SetBytes(x)
			case gae.DSByteString:
				v.SetBytes([]byte(x))
			default:
				panic(fmt.Errorf("helper: impossible: %s", typeMismatchReason(pVal, v)))
			}
		default:
			panic(fmt.Errorf("helper: impossible: %s", typeMismatchReason(pVal, v)))
		}
	}
	if slice.IsValid() {
		slice.Set(reflect.Append(slice, v))
	}
	return ""
}

func (p *structPLS) Save() (gae.DSPropertyMap, error) {
	ret := gae.DSPropertyMap{}
	idxCount := 0
	if err := p.save(ret, &idxCount, "", false); err != nil {
		return nil, err
	}
	return ret, nil
}

func (p *structPLS) save(propMap gae.DSPropertyMap, idxCount *int, prefix string, noIndex bool) (err error) {
	if err = p.Problem(); err != nil {
		return
	}

	saveProp := func(name string, ni bool, v reflect.Value, st *structTag) (err error) {
		if st.substructCodec != nil {
			return (&structPLS{v, st.substructCodec}).save(propMap, idxCount, name, ni)
		}

		prop := gae.DSProperty{}
		if st.convert {
			prop, err = v.Addr().Interface().(gae.DSPropertyConverter).ToDSProperty()
		} else {
			err = prop.SetValue(v.Interface(), ni)
		}
		if err != nil {
			return err
		}
		propMap[name] = append(propMap[name], prop)
		if !prop.NoIndex() {
			*idxCount++
			if *idxCount > maxIndexedProperties {
				return errors.New("gae: too many indexed properties")
			}
		}
		return nil
	}

	for i, st := range p.c.byIndex {
		if st.name == "-" {
			continue
		}
		name := st.name
		if prefix != "" {
			name = prefix + name
		}
		v := p.o.Field(i)
		noIndex1 := noIndex || st.noIndex
		if st.isSlice {
			for j := 0; j < v.Len(); j++ {
				if err := saveProp(name, noIndex1, v.Index(j), &st); err != nil {
					return err
				}
			}
		} else {
			if err := saveProp(name, noIndex1, v, &st); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *structPLS) GetSpecial(key string) (val string, current interface{}, err error) {
	if err = p.Problem(); err != nil {
		return
	}
	idx, ok := p.c.bySpecial[key]
	if !ok {
		err = gae.ErrDSSpecialFieldUnset
		return
	}
	val = p.c.byIndex[idx].specialVal
	f := p.o.Field(idx)
	if f.CanSet() {
		current = f.Interface()
	}
	return
}

func (p *structPLS) SetSpecial(key string, val interface{}) (err error) {
	if err = p.Problem(); err != nil {
		return
	}
	idx, ok := p.c.bySpecial[key]
	if !ok {
		return gae.ErrDSSpecialFieldUnset
	}
	defer func() {
		pv := recover()
		if pv != nil && err == nil {
			err = fmt.Errorf("gae/helper: cannot set special %q: %s", key, pv)
		}
	}()
	p.o.Field(idx).Set(reflect.ValueOf(val))
	return nil
}

func (p *structPLS) Problem() error { return p.c.problem }

var (
	// The RWMutex is chosen intentionally, as the majority of access to the
	// structCodecs map will be in parallel and will be to read an existing codec.
	// There's no reason to serialize goroutines on every
	// gae.RawDatastore.{Get,Put}{,Multi} call.
	structCodecsMutex sync.RWMutex
	structCodecs      = map[reflect.Type]*structCodec{}
)

// validPropertyName returns whether name consists of one or more valid Go
// identifiers joined by ".".
func validPropertyName(name string) bool {
	if name == "" {
		return false
	}
	for _, s := range strings.Split(name, ".") {
		if s == "" {
			return false
		}
		first := true
		for _, c := range s {
			if first {
				first = false
				if c != '_' && !unicode.IsLetter(c) {
					return false
				}
			} else {
				if c != '_' && !unicode.IsLetter(c) && !unicode.IsDigit(c) {
					return false
				}
			}
		}
	}
	return true
}

var (
	errRecursiveStruct = fmt.Errorf("(internal): struct type is recursively defined")
)

func getStructCodecLocked(t reflect.Type) (c *structCodec) {
	if c, ok := structCodecs[t]; ok {
		return c
	}

	me := func(fmtStr string, args ...interface{}) error {
		return fmt.Errorf(fmtStr, args...)
	}

	c = &structCodec{
		byIndex:   make([]structTag, t.NumField()),
		byName:    make(map[string]int, t.NumField()),
		bySpecial: make(map[string]int, t.NumField()),
		problem:   errRecursiveStruct, // we'll clear this later if it's not recursive
	}
	defer func() {
		// If the codec has a problem, free up the indexes
		if c.problem != nil {
			c.byIndex = nil
			c.byName = nil
			c.bySpecial = nil
		}
	}()
	structCodecs[t] = c

	for i := range c.byIndex {
		st := &c.byIndex[i]
		f := t.Field(i)
		name := f.Tag.Get("gae")
		opts := ""
		if i := strings.Index(name, ","); i != -1 {
			name, opts = name[:i], name[i+1:]
		}
		switch {
		case name == "":
			if !f.Anonymous {
				name = f.Name
			}
		case name[0] == '$':
			name = name[1:]
			if _, ok := c.bySpecial[name]; ok {
				c.problem = me("special field %q set multiple times", "$"+name)
				return
			}
			c.bySpecial[name] = i
			st.specialVal = opts
			fallthrough
		case name == "-":
			st.name = "-"
			continue
		default:
			if !validPropertyName(name) {
				c.problem = me("struct tag has invalid property name: %q", name)
				return
			}
		}
		if f.PkgPath != "" { // field is unexported, so don't bother doing more.
			st.name = "-"
			continue
		}

		substructType := reflect.Type(nil)
		ft := f.Type
		if reflect.PtrTo(ft).Implements(typeOfDSPropertyConverter) {
			st.convert = true
		} else {
			switch f.Type.Kind() {
			case reflect.Struct:
				if ft != typeOfTime && ft != typeOfGeoPoint {
					substructType = ft
				}
			case reflect.Slice:
				if reflect.PtrTo(ft.Elem()).Implements(typeOfDSPropertyConverter) {
					st.convert = true
				} else if ft.Elem().Kind() == reflect.Struct {
					substructType = ft.Elem()
				}
				st.isSlice = ft.Elem().Kind() != reflect.Uint8
				c.hasSlice = c.hasSlice || st.isSlice
			case reflect.Interface:
				if ft != typeOfDSKey {
					c.problem = me("field %q has non-concrete interface type %s",
						f.Name, f.Type)
					return
				}
			}
		}

		if substructType != nil {
			sub := getStructCodecLocked(substructType)
			if sub.problem != nil {
				if sub.problem == errRecursiveStruct {
					c.problem = me("field %q is recursively defined", f.Name)
				} else {
					c.problem = me("field %q has problem: %s", f.Name, sub.problem)
				}
				return
			}
			st.substructCodec = sub
			if st.isSlice && sub.hasSlice {
				c.problem = me(
					"flattening nested structs leads to a slice of slices: field %q",
					f.Name)
				return
			}
			c.hasSlice = c.hasSlice || sub.hasSlice
			if name != "" {
				name += "."
			}
			for relName := range sub.byName {
				absName := name + relName
				if _, ok := c.byName[absName]; ok {
					c.problem = me("struct tag has repeated property name: %q", absName)
					return
				}
				c.byName[absName] = i
			}
		} else {
			if !st.convert { // check the underlying static type of the field
				t := ft
				if st.isSlice {
					t = t.Elem()
				}
				v := reflect.New(t).Elem().Interface()
				v, _ = gae.DSUpconvertUnderlyingType(v, t)
				if _, err := gae.DSPropertyTypeOf(v, false); err != nil {
					c.problem = me("field %q has invalid type: %s", name, ft)
					return
				}
			}

			if _, ok := c.byName[name]; ok {
				c.problem = me("struct tag has repeated property name: %q", name)
				return
			}
			c.byName[name] = i
		}
		st.name = name
		st.noIndex = opts == "noindex"
	}
	if c.problem == errRecursiveStruct {
		c.problem = nil
	}
	return
}
