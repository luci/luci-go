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

// HEAVILY adapted from github.com/golang/appengine/datastore

package datastore

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"go.chromium.org/luci/common/errors"
)

// Entities with more than this many indexed properties will not be saved.
const maxIndexedProperties = 20000

type structTag struct {
	name           string
	idxSetting     IndexSetting
	isSlice        bool
	substructCodec *structCodec
	convert        bool
	metaVal        interface{}
	isExtra        bool
	canSet         bool
}

type structCodec struct {
	byMeta    map[string]int
	byName    map[string]int
	bySpecial map[string]int

	byIndex  []structTag
	hasSlice bool
	problem  error
}

type structPLS struct {
	o   reflect.Value
	c   *structCodec
	mgs MetaGetterSetter
}

var _ PropertyLoadSaver = (*structPLS)(nil)

// typeMismatchReason returns a string explaining why the property p could not
// be stored in an entity field of type v.Type().
func typeMismatchReason(val interface{}, v reflect.Value) string {
	entityType := reflect.TypeOf(val)
	return fmt.Sprintf("type mismatch: %s versus %v", entityType, v.Type())
}

func (p *structPLS) Load(propMap PropertyMap) error {
	convFailures := errors.MultiError(nil)

	useExtra := false
	extra := (*PropertyMap)(nil)
	if i, ok := p.c.bySpecial["extra"]; ok {
		useExtra = true
		f := p.c.byIndex[i]
		if f.canSet {
			extra = p.o.Field(i).Addr().Interface().(*PropertyMap)
		}
	}
	t := reflect.Type(nil)
	for name, pdata := range propMap {
		pslice := pdata.Slice()
		requireSlice := len(pslice) > 1
		for i, prop := range pslice {
			if reason := loadInner(p.c, p.o, i, name, prop, requireSlice); reason != "" {
				if useExtra {
					if extra != nil {
						if *extra == nil {
							*extra = make(PropertyMap, 1)
						}
						(*extra)[name] = pslice
					}
					break // go to the next property in propMap
				} else {
					if t == nil {
						t = p.o.Type()
					}
					convFailures = append(convFailures, &ErrFieldMismatch{
						StructType: t,
						FieldName:  name,
						Reason:     reason,
					})
				}
			}
		}
	}

	if len(convFailures) > 0 {
		return convFailures
	}

	return nil
}

func loadInner(codec *structCodec, structValue reflect.Value, index int, name string, p Property, requireSlice bool) string {
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
		if conv, ok := a.Interface().(PropertyConverter); ok {
			err := conv.FromProperty(p)
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

	if ret, ok := doConversion(v); ok {
		if ret != "" {
			return ret
		}
	} else {
		knd := v.Kind()

		project := PTNull
		overflow := (func(interface{}) bool)(nil)
		set := (func(interface{}))(nil)

		switch knd {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			project = PTInt
			overflow = func(x interface{}) bool { return v.OverflowInt(x.(int64)) }
			set = func(x interface{}) { v.SetInt(x.(int64)) }
		case reflect.Uint8, reflect.Uint16, reflect.Uint32:
			project = PTInt
			overflow = func(x interface{}) bool {
				xi := x.(int64)
				return xi < 0 || v.OverflowUint(uint64(xi))
			}
			set = func(x interface{}) { v.SetUint(uint64(x.(int64))) }
		case reflect.Bool:
			project = PTBool
			set = func(x interface{}) { v.SetBool(x.(bool)) }
		case reflect.String:
			project = PTString
			set = func(x interface{}) { v.SetString(x.(string)) }
		case reflect.Float32, reflect.Float64:
			project = PTFloat
			overflow = func(x interface{}) bool { return v.OverflowFloat(x.(float64)) }
			set = func(x interface{}) { v.SetFloat(x.(float64)) }
		case reflect.Ptr:
			project = PTKey
			set = func(x interface{}) {
				if k, ok := x.(*Key); ok {
					v.Set(reflect.ValueOf(k))
				}
			}
		case reflect.Struct:
			switch v.Type() {
			case typeOfTime:
				project = PTTime
				set = func(x interface{}) { v.Set(reflect.ValueOf(x)) }
			case typeOfGeoPoint:
				project = PTGeoPoint
				set = func(x interface{}) { v.Set(reflect.ValueOf(x)) }
			default:
				panic(fmt.Errorf("helper: impossible: %s", typeMismatchReason(p.Value(), v)))
			}
		case reflect.Slice:
			project = PTBytes
			set = func(x interface{}) {
				v.SetBytes(reflect.ValueOf(x).Bytes())
			}
		default:
			panic(fmt.Errorf("helper: impossible: %s", typeMismatchReason(p.Value(), v)))
		}

		pVal, err := p.Project(project)
		if err != nil {
			return typeMismatchReason(p.Value(), v)
		}
		if overflow != nil && overflow(pVal) {
			return fmt.Sprintf("value %v overflows struct field of type %v", pVal, v.Type())
		}
		set(pVal)
	}
	if slice.IsValid() {
		slice.Set(reflect.Append(slice, v))
	}
	return ""
}

func (p *structPLS) Save(withMeta bool) (PropertyMap, error) {
	ret := PropertyMap(nil)
	if withMeta {
		if p.mgs != nil {
			ret = p.mgs.GetAllMeta()
		} else {
			ret = p.GetAllMeta()
		}
	} else {
		ret = make(PropertyMap, len(p.c.byName))
	}
	if _, err := p.save(ret, "", nil, ShouldIndex); err != nil {
		return nil, err
	}
	return ret, nil
}

func (p *structPLS) getDefaultKind() string {
	if !p.o.IsValid() {
		return ""
	}
	return p.o.Type().Name()
}

func (p *structPLS) save(propMap PropertyMap, prefix string, parentST *structTag, is IndexSetting) (idxCount int, err error) {
	saveProp := func(name string, si IndexSetting, v reflect.Value, st *structTag) (err error) {
		if st.substructCodec != nil {
			count, err := (&structPLS{v, st.substructCodec, nil}).save(propMap, name, st, si)
			if err == nil {
				idxCount += count
				if idxCount > maxIndexedProperties {
					err = errors.New("gae: too many indexed properties")
				}
			}
			return err
		}

		prop := Property{}
		if st.convert {
			prop, err = v.Addr().Interface().(PropertyConverter).ToProperty()
		} else {
			err = prop.SetValue(v.Interface(), si)
		}
		if err != nil {
			return err
		}

		// If we're a slice, or we are members in a slice, then use a PropertySlice.
		if st.isSlice || (parentST != nil && parentST.isSlice) {
			var pslice PropertySlice
			if pdata := propMap[name]; pdata != nil {
				pslice = pdata.(PropertySlice)
			}
			propMap[name] = append(pslice, prop)
		} else {
			if _, ok := propMap[name]; ok {
				return errors.New("non-slice property adding multiple PropertyMap entries")
			}
			propMap[name] = prop
		}

		if prop.IndexSetting() == ShouldIndex {
			idxCount++
			if idxCount > maxIndexedProperties {
				return errors.New("gae: too many indexed properties")
			}
		}
		return nil
	}

	for i, st := range p.c.byIndex {
		if st.name == "-" || st.isExtra {
			continue
		}
		name := st.name
		if prefix != "" {
			name = prefix + name
		}
		v := p.o.Field(i)
		is1 := is
		if st.idxSetting == NoIndex {
			is1 = NoIndex
		}
		if st.isSlice {
			for j := 0; j < v.Len(); j++ {
				if err = saveProp(name, is1, v.Index(j), &st); err != nil {
					err = fmt.Errorf("gae: failed to save slice field %q: %v", name, err)
					return
				}
			}
		} else {
			if err = saveProp(name, is1, v, &st); err != nil {
				err = fmt.Errorf("gae: failed to save single field %q: %v", name, err)
				return
			}
		}
	}

	if i, ok := p.c.bySpecial["extra"]; ok {
		if p.c.byIndex[i].name != "-" {
			for fullName, vals := range p.o.Field(i).Interface().(PropertyMap) {
				if _, ok := propMap[fullName]; !ok {
					propMap[fullName] = vals
				}
			}
		}
	}

	return
}

func (p *structPLS) GetMeta(key string) (interface{}, bool) {
	if idx, ok := p.c.byMeta[key]; ok {
		if val, ok := p.getMetaFor(idx); ok {
			return val, true
		}
	} else if key == "kind" {
		return p.getDefaultKind(), true
	}
	return nil, false
}

func (p *structPLS) getMetaFor(idx int) (interface{}, bool) {
	st := p.c.byIndex[idx]
	val := st.metaVal
	if st.canSet {
		f := p.o.Field(idx)
		if st.convert {
			prop, err := f.Addr().Interface().(PropertyConverter).ToProperty()
			if err != nil {
				return nil, false
			}
			return prop.Value(), true
		}

		if !reflect.DeepEqual(reflect.Zero(f.Type()).Interface(), f.Interface()) {
			val = f.Interface()
			if bf, ok := val.(Toggle); ok {
				val = bf == On // true if On, otherwise false
			} else {
				val = UpconvertUnderlyingType(val)
			}
		}
	}
	return val, true
}

func (p *structPLS) GetAllMeta() PropertyMap {
	needKind := true
	ret := make(PropertyMap, len(p.c.byMeta)+1)
	for k, idx := range p.c.byMeta {
		if val, ok := p.getMetaFor(idx); ok {
			p := Property{}
			if err := p.SetValue(val, NoIndex); err != nil {
				continue
			}
			ret["$"+k] = p
		}
	}
	if needKind {
		if _, ok := p.c.byMeta["kind"]; !ok {
			ret["$kind"] = MkPropertyNI(p.getDefaultKind())
		}
	}
	return ret
}

func (p *structPLS) SetMeta(key string, val interface{}) bool {
	idx, ok := p.c.byMeta[key]
	if !ok {
		return false
	}
	st := p.c.byIndex[idx]
	if !st.canSet {
		return false
	}
	if st.convert {
		err := p.o.Field(idx).Addr().Interface().(PropertyConverter).FromProperty(
			MkPropertyNI(val))
		return err == nil
	}

	val = UpconvertUnderlyingType(val)

	// setting a Toggle
	if b, ok := val.(bool); ok {
		if b {
			val = On
		} else {
			val = Off
		}
	}
	f := p.o.Field(idx)
	if val == nil {
		f.Set(reflect.Zero(f.Type()))
	} else {
		value := reflect.ValueOf(val)
		switch f.Kind() {
		case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			intVal := value.Int()
			if f.OverflowInt(intVal) {
				return false
			}
			f.SetInt(intVal)
		case reflect.Uint8, reflect.Uint16, reflect.Uint32:
			if f.Type() != typeOfToggle {
				intVal := value.Int()
				if intVal < 0 || f.OverflowUint(uint64(intVal)) {
					return false
				}
				f.SetUint(uint64(intVal))
				break
			}
			fallthrough
		default:
			f.Set(value.Convert(f.Type()))
		}
	}
	return true
}

var (
	// The RWMutex is chosen intentionally, as the majority of access to the
	// structCodecs map will be in parallel and will be to read an existing codec.
	// There's no reason to serialize goroutines on every
	// gae.Interface.{Get,Put}{,Multi} call.
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
		byMeta:    make(map[string]int, t.NumField()),
		bySpecial: make(map[string]int, 1),

		problem: errRecursiveStruct, // we'll clear this later if it's not recursive
	}
	defer func() {
		// If the codec has a problem, free up the indexes
		if c.problem != nil {
			c.byIndex = nil
			c.byName = nil
			c.byMeta = nil
		}
	}()
	structCodecs[t] = c

	for i := range c.byIndex {
		st := &c.byIndex[i]
		f := t.Field(i)
		ft := f.Type

		name := f.Tag.Get("gae")
		opts := ""
		if i := strings.Index(name, ","); i != -1 {
			name, opts = name[:i], name[i+1:]
		}
		st.canSet = f.PkgPath == "" // blank == exported
		if opts == "extra" {
			if _, ok := c.bySpecial["extra"]; ok {
				c.problem = me("struct has multiple fields tagged as 'extra'")
				return
			}
			if name != "" && name != "-" {
				c.problem = me("struct 'extra' field has invalid name %s, expecing `` or `-`", name)
				return
			}
			if ft != typeOfPropertyMap {
				c.problem = me("struct 'extra' field has invalid type %s, expecing PropertyMap", ft)
				return
			}
			st.isExtra = true
			st.name = name
			c.bySpecial["extra"] = i
			continue
		}
		st.convert = reflect.PtrTo(ft).Implements(typeOfPropertyConverter)
		switch {
		case name == "":
			if !f.Anonymous {
				name = f.Name
			}
		case name[0] == '$':
			name = name[1:]
			if _, ok := c.byMeta[name]; ok {
				c.problem = me("meta field %q set multiple times", "$"+name)
				return
			}
			c.byMeta[name] = i
			if !st.convert {
				mv, err := convertMeta(opts, ft)
				if err != nil {
					c.problem = me("meta field %q has bad type: %s", "$"+name, err)
					return
				}
				st.metaVal = mv
			}
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
		if !st.canSet {
			st.name = "-"
			continue
		}

		substructType := reflect.Type(nil)
		if !st.convert {
			switch ft.Kind() {
			case reflect.Struct:
				if ft != typeOfTime && ft != typeOfGeoPoint {
					substructType = ft
				}
			case reflect.Slice:
				if reflect.PtrTo(ft.Elem()).Implements(typeOfPropertyConverter) {
					st.convert = true
				} else if ft.Elem().Kind() == reflect.Struct {
					substructType = ft.Elem()
				}
				st.isSlice = ft.Elem().Kind() != reflect.Uint8
				c.hasSlice = c.hasSlice || st.isSlice
			case reflect.Interface:
				c.problem = me("field %q has non-concrete interface type %s",
					f.Name, ft)
				return
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
				v := UpconvertUnderlyingType(reflect.New(t).Elem().Interface())
				if _, err := PropertyTypeOf(v, false); err != nil {
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
		if opts == "noindex" {
			st.idxSetting = NoIndex
		}
	}
	if c.problem == errRecursiveStruct {
		c.problem = nil
	}
	return
}

func convertMeta(val string, t reflect.Type) (interface{}, error) {
	switch t.Kind() {
	case reflect.String:
		return val, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if val == "" {
			return int64(0), nil
		}
		return strconv.ParseInt(val, 10, 64)
	case reflect.Uint8, reflect.Uint16, reflect.Uint32:
		if t == typeOfToggle { // special case this
			break
		}
		if val == "" {
			return int64(0), nil
		}
		ret, err := strconv.ParseUint(val, 10, 32)
		return int64(ret), err
	}
	switch t {
	case typeOfKey:
		if val != "" {
			return nil, fmt.Errorf("key field is not allowed to have a default: %q", val)
		}
		return nil, nil
	case typeOfToggle:
		switch val {
		case "on", "On", "true":
			return true, nil
		case "off", "Off", "false":
			return false, nil
		}
		return nil, fmt.Errorf("Toggle field has bad/missing default, got %q", val)
	}
	return nil, fmt.Errorf("helper: meta field with bad type/value %s/%q", t, val)
}
