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

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/gae/internal/zlib"
)

// Entities with more than this many indexed properties will not be saved.
const maxIndexedProperties = 20000

type convertMethod byte

const (
	convertDefault convertMethod = iota
	// field implements PropertyConverter
	convertProp
	// field implements proto.Message
	convertProto
	// field is a "local structured property"
	convertLSP
)

type structTag struct {
	name           string
	idxSetting     IndexSetting
	isSlice        bool
	substructCodec *structCodec
	convertMethod  convertMethod

	metaVal  any
	isExtra  bool
	exported bool

	protoOption protoOption
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
func typeMismatchReason(val any, v reflect.Value) string {
	entityType := reflect.TypeOf(val)
	return fmt.Sprintf("type mismatch: %s versus %v", entityType, v.Type())
}

func (p *structPLS) Load(propMap PropertyMap) error {
	return p.loadWithMeta(propMap, false)
}

// loadWithMeta loads PropertyMap into the struct fields.
//
// `hasMeta` controls what to do with property map fields that carry meta
// properties like `$key`. If `hasMeta` is true, they will be loaded into
// corresponding meta fields using SetMeta(...). If `hasMeta` is false, no
// meta fields are expected (their presence will result in an error).
//
// `hasMeta` true is used when loading nested entities that may have their keys
// populated. For top-level entities meta fields are set by the caller using
// a different mechanism.
func (p *structPLS) loadWithMeta(propMap PropertyMap, hasMeta bool) error {
	convFailures := errors.MultiError(nil)

	p.resetBeforeLoad()

	useExtra := false
	extra := (*PropertyMap)(nil)
	if i, ok := p.c.bySpecial["extra"]; ok {
		useExtra = true
		f := p.c.byIndex[i]
		if f.exported {
			extra = p.o.Field(i).Addr().Interface().(*PropertyMap)
		}
	}
	t := reflect.Type(nil)
	for name, pdata := range propMap {
		if hasMeta && name[0] == '$' {
			if prop, ok := pdata.(Property); ok {
				p.SetMeta(name[1:], prop.Value())
			} else {
				convFailures = append(convFailures, &ErrFieldMismatch{
					StructType: t,
					FieldName:  name,
					Reason:     "trying to load a property slice into a meta field",
				})
			}
			continue
		}
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

// resetBeforeLoad resets exported to Datastore fields of the struct,
// recursively.
func (p *structPLS) resetBeforeLoad() {
	var reset func(codec *structCodec, structValue reflect.Value)
	reset = func(codec *structCodec, structValue reflect.Value) {
		for fieldIndex, tag := range codec.byIndex {
			field := structValue.Field(fieldIndex)
			switch {
			case !tag.exported || tag.name == "-":
				// Either not exported field of a struct,
				// or not exported to Datastore (e.g. `gae:"-"`),
				// or special $id or $parent, whose codec's tag.name is also "-".
			case tag.substructCodec != nil && !tag.isSlice:
				// Recurse. Applies to flattened structs as well as "local structured
				// properties", which are also represented by structs.
				reset(tag.substructCodec, field)
			default:
				field.Set(reflect.Zero(field.Type()))
			}
		}
	}

	reset(p.c, p.o)
}

func loadInner(codec *structCodec, structValue reflect.Value, index int, name string, p Property, requireSlice bool) string {
	var v reflect.Value
	var convertMethod convertMethod
	var substructCodec *structCodec
	var protoOption protoOption

	// Traverse a struct's struct-typed fields.
	for {
		fieldIndex, ok := codec.byName[name]
		if !ok {
			return "no such struct field"
		}
		v = structValue.Field(fieldIndex)

		st := codec.byIndex[fieldIndex]

		if st.convertMethod == convertDefault && st.substructCodec != nil {
			// The property is a part of a nested struct or a slice of nested structs.
			// By construction, `st` represents the struct field itself (let's call
			// it "S"). Here the property name is something like "S.X.Y.Z" and
			// `st.name` is "S." (since the codec constructor appends "." to struct
			// fields). Recurse deeper to get "X.Y.Z" and the codec that has it as
			// a field.
			name = name[len(st.name):]
			codec = st.substructCodec
			// Move the reflection pointer accordingly as well. Here `requireSlice` is
			// true if `p` came from a slice. It means it must map onto a repeated
			// Go field at some point. In a chain of nested structs only one link is
			// allowed to be repeated (because datastore doesn't support `[][]Type`).
			// When we traverse such repeated link, we can "disarm" `requireSlice`
			// check right away. Note that per `st` construction, this can happen only
			// once (i.e. on the subsequent iterations of this loop `v` will never be
			// a slice).
			if v.Kind() == reflect.Slice {
				for v.Len() <= index {
					v.Set(reflect.Append(v, reflect.New(v.Type().Elem()).Elem()))
				}
				structValue = v.Index(index)
				requireSlice = false
			} else {
				structValue = v
			}
			continue
		}

		// The property doesn't have any non-opaque internal structure. Can't go
		// deeper. Extract relevant properties from `st` and proceed to loading
		// the value.
		convertMethod = st.convertMethod
		substructCodec = st.substructCodec
		protoOption = st.protoOption
		break
	}

	// Try to apply a property converter codec if the field has any. Do it before
	// the slice check, because a slice *may* itself implement PropertyConverter
	// interface. Note that other convertMethod types don't directly apply to
	// slice-valued fields, e.g. we don't need to check convertProto here.
	if convertMethod == convertProp {
		if conv, ok := v.Addr().Interface().(PropertyConverter); ok {
			if err := conv.FromProperty(p); err != nil {
				return err.Error()
			}
			return ""
		}
	}

	// If the field is slice-valued, we'll deserialize the property into a new
	// variable and then append it to the slice in the end.
	var slice reflect.Value
	if v.Kind() == reflect.Slice && v.Type().Elem().Kind() != reflect.Uint8 {
		slice = v
		v = reflect.New(v.Type().Elem()).Elem()
	} else if requireSlice {
		return "multiple-valued property requires a slice field type"
	}

	// Here `v` is a singular value. It is either some elementary type or
	// something that needs a property deserializer of some sort.
	switch convertMethod {
	case convertDefault:
		loader := elementaryLoader(v)
		if loader.project == PTNull {
			panic(fmt.Errorf("helper: impossible: %s", typeMismatchReason(p.Value(), v)))
		}
		pVal, err := p.Project(loader.project)
		if err != nil {
			return typeMismatchReason(p.Value(), v)
		}
		if loader.overflow != nil && loader.overflow(pVal) {
			return fmt.Sprintf("value %v overflows struct field of type %v", pVal, v.Type())
		}
		loader.set(pVal)

	case convertProto:
		if err := protoFromProperty(v, p, protoOption); err != nil {
			return err.Error()
		}

	case convertProp:
		// It *must* implement PropertyConverter at this point. This is the only way
		// convertMethod can be convertProp.
		conv := v.Addr().Interface().(PropertyConverter)
		if err := conv.FromProperty(p); err != nil {
			return err.Error()
		}

	case convertLSP:
		// It is possible we get a blob here produced by Python. It is either
		// a compressed blob when using LocalStructuredProperty(compressed=True), or
		// an uncompressed one when using LocalStructuredProperty(compressed=False),
		// but inside *some other* LocalStructuredProperty(compressed=True).
		//
		// Top-level uncompressed LSPs are stored as nested property maps, even when
		// written from Python: there appears to be some compatibility layer in
		// Cloud Datastore backend that converts blobs produced by Python into
		// properly expanded property maps (aka entity-valued properties). But this
		// magic doesn't know how look inside compressed blobs.
		var pm PropertyMap
		var hasMeta bool
		if blob, ok := p.Value().([]byte); ok {
			var err error
			if zlib.HasZlibHeader(blob) {
				if blob, err = zlib.DecodeAll(blob, nil); err != nil {
					return fmt.Sprintf("decompressing legacy LSP: %s", err)
				}
			}
			if pm, err = loadLegacyLSP(blob); err != nil {
				return fmt.Sprintf("loading legacy LSP: %s", err)
			}
		} else if pmap, ok := p.Value().(PropertyMap); ok {
			pm = pmap
			hasMeta = true
		} else if p.Type() != PTNull {
			return fmt.Sprintf("expecting a property map or null, but got %s", p.Type())
		}
		if err := (&structPLS{o: v, c: substructCodec}).loadWithMeta(pm, hasMeta); err != nil {
			return fmt.Sprintf("loading LSP: %s", err)
		}

	default:
		panic(fmt.Errorf("helper: impossible method: %d", convertMethod))
	}

	// If this was an item of a repeated field, append it to the resulting slice.
	if slice.IsValid() {
		slice.Set(reflect.Append(slice, v))
	}

	return ""
}

// fieldLoader describes how to write a Property into a reflect.Value.
//
// Zero value (in particular project == PTNull) means the property type is
// unrecognized.
type fieldLoader struct {
	// project is a type to cast Property into before reading it.
	project PropertyType
	// overflow checks if the value will overflow the target reflect.Value.
	overflow func(any) bool
	// set assigned the value to the target reflect.Value
	set func(any)
}

// Understands how to convert properties into elementary values.
func elementaryLoader(v reflect.Value) fieldLoader {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fieldLoader{
			project:  PTInt,
			overflow: func(x any) bool { return v.OverflowInt(x.(int64)) },
			set:      func(x any) { v.SetInt(x.(int64)) },
		}
	case reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return fieldLoader{
			project: PTInt,
			overflow: func(x any) bool {
				xi := x.(int64)
				return xi < 0 || v.OverflowUint(uint64(xi))
			},
			set: func(x any) { v.SetUint(uint64(x.(int64))) },
		}
	case reflect.Bool:
		return fieldLoader{
			project: PTBool,
			set:     func(x any) { v.SetBool(x.(bool)) },
		}
	case reflect.String:
		return fieldLoader{
			project: PTString,
			set:     func(x any) { v.SetString(x.(string)) },
		}
	case reflect.Float32, reflect.Float64:
		return fieldLoader{
			project:  PTFloat,
			overflow: func(x any) bool { return v.OverflowFloat(x.(float64)) },
			set:      func(x any) { v.SetFloat(x.(float64)) },
		}
	case reflect.Ptr:
		return fieldLoader{
			project: PTKey,
			set: func(x any) {
				if k, ok := x.(*Key); ok {
					v.Set(reflect.ValueOf(k))
				}
			},
		}
	case reflect.Struct:
		switch v.Type() {
		case typeOfTime:
			return fieldLoader{
				project: PTTime,
				set:     func(x any) { v.Set(reflect.ValueOf(x)) },
			}
		case typeOfGeoPoint:
			return fieldLoader{
				project: PTGeoPoint,
				set:     func(x any) { v.Set(reflect.ValueOf(x)) },
			}
		default:
			return fieldLoader{} // will trigger an error
		}
	case reflect.Slice:
		return fieldLoader{
			project: PTBytes,
			set: func(x any) {
				v.SetBytes(reflect.ValueOf(x).Bytes())
			},
		}
	default:
		return fieldLoader{} // will trigger an error
	}
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
	if _, err := p.save(ret, "", false, ShouldIndex); err != nil {
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

func (p *structPLS) save(propMap PropertyMap, prefix string, inSlice bool, is IndexSetting) (idxCount int, err error) {
	saveProp := func(name string, si IndexSetting, v reflect.Value, st *structTag) (err error) {
		prop := Property{}
		switch {
		case st.convertMethod == convertDefault && st.substructCodec != nil:
			// This is a nested struct.
			count, err := (&structPLS{v, st.substructCodec, nil}).save(propMap, name, inSlice || st.isSlice, si)
			if err == nil {
				idxCount += count
				if idxCount > maxIndexedProperties {
					err = errors.New("gae: too many indexed properties")
				}
			}
			return err

		case st.convertMethod == convertDefault && st.substructCodec == nil:
			// This is some elementary value.
			err = prop.SetValue(v.Interface(), si)
			if err != nil {
				return err
			}

		case st.convertMethod == convertLSP:
			// This is a "local structured property". Need to save it into its own
			// property map, with meta fields populated (in case it has a `$key` or
			// similar). If this field opted out of indexing, avoid indexing nested
			// properties as well.
			is1 := is
			if st.idxSetting == NoIndex {
				is1 = NoIndex
			}
			innerPLS := &structPLS{o: v, c: st.substructCodec}
			pmap := innerPLS.GetAllMeta()
			if _, err := innerPLS.save(pmap, "", false, is1); err != nil {
				return fmt.Errorf("saving LSP: %w", err)
			}
			err = prop.SetValue(pmap, is1)
			if err != nil {
				return err
			}

		case st.convertMethod == convertProp:
			// The field knows how to convert itself into a property.
			prop, err = v.Addr().Interface().(PropertyConverter).ToProperty()
			if err != nil {
				if errors.Is(err, ErrSkipProperty) {
					return nil
				}
				return err
			}

		case st.convertMethod == convertProto:
			// Don't emit a property for a nil proto message, such that load won't
			// set proto field value to an empty message.
			if v.IsNil() {
				return nil
			}
			prop, err = protoToProperty(v.Interface().(proto.Message), st.protoOption)
			if err != nil {
				return err
			}

		default:
			panic(fmt.Errorf("unknown convertMethod: %d", st.convertMethod))
		}

		// If we're a slice, or we are members in a slice (perhaps recursively),
		// then append to a PropertySlice with the corresponding name. That way
		// visiting a repeated nested struct will populate all flattened property
		// slices correctly.
		if st.isSlice || inSlice {
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
			// TODO(vadimsh): This check is probably broken or makes no sense when
			// using nested indexed entities. It will count `1` for them instead of
			// `NumberOfFieldsInside`. But it is not clear how datastore itself counts
			// indexes of nested entities.
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

func (p *structPLS) GetMeta(key string) (any, bool) {
	if idx, ok := p.c.byMeta[key]; ok {
		if val, ok := p.getMetaFor(idx); ok {
			return val, true
		}
	} else if key == "kind" {
		return p.getDefaultKind(), true
	}
	return nil, false
}

func (p *structPLS) getMetaFor(idx int) (any, bool) {
	st := p.c.byIndex[idx]
	val := st.metaVal

	if !st.exported {
		return val, true
	}

	f := p.o.Field(idx)
	switch st.convertMethod {
	case convertDefault:
		// Carry on.
	case convertProp:
		prop, err := f.Addr().Interface().(PropertyConverter).ToProperty()
		if err != nil {
			return nil, false
		}
		return prop.Value(), true
	case convertProto:
		if f.IsNil() {
			return nil, false
		}
		prop, err := protoToProperty(f.Interface().(proto.Message), st.protoOption)
		if err != nil {
			return nil, false
		}
		return prop.Value(), true
	default:
		panic(fmt.Errorf("impossible convertMethod: %d", st.convertMethod))
	}

	if !reflect.DeepEqual(reflect.Zero(f.Type()).Interface(), f.Interface()) {
		val = f.Interface()
		if bf, ok := val.(Toggle); ok {
			val = bf == On // true if On, otherwise false
		} else {
			val = UpconvertUnderlyingType(val)
		}
	}
	return val, true
}

func (p *structPLS) GetAllMeta() PropertyMap {
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
	if _, ok := p.c.byMeta["kind"]; !ok {
		ret["$kind"] = MkPropertyNI(p.getDefaultKind())
	}
	return ret
}

func (p *structPLS) SetMeta(key string, val any) bool {
	idx, ok := p.c.byMeta[key]
	if !ok {
		return false
	}
	st := p.c.byIndex[idx]
	if !st.exported {
		return false
	}

	switch st.convertMethod {
	case convertDefault:
		// Carry on.
	case convertProp:
		err := p.o.Field(idx).Addr().Interface().(PropertyConverter).FromProperty(
			MkPropertyNI(val))
		return err == nil
	case convertProto:
		err := protoFromProperty(p.o.Field(idx), MkPropertyNI(val), st.protoOption)
		return err == nil
	default:
		panic(fmt.Errorf("impossible convertMethod: %d", st.convertMethod))
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

func isStruct(t reflect.Type) bool {
	return t.Kind() == reflect.Struct && t != typeOfTime && t != typeOfGeoPoint
}

func getStructCodecLocked(t reflect.Type) (c *structCodec) {
	if c, ok := structCodecs[t]; ok {
		return c
	}

	me := func(fmtStr string, args ...any) error {
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

		st.exported = f.PkgPath == "" // true for UpperCasePublicFields

		tag := f.Tag.Get("gae")
		if t := string(f.Tag); tag == "" && strings.Contains(t, `gae:`) && !strings.Contains(t, `gae:"`) {
			// Catch typos like
			//   struct { F int `gae:f,noindex` }
			// which should be
			//   struct { F int `gae:"f,noindex"` }
			c.problem = me("struct tag is invalid: %q (did you mean `gae:\"...\"`?)", f.Tag)
			return
		}

		opts := "" // statements like "noindex"
		st.name, opts, _ = strings.Cut(tag, ",")

		// The field tagged as "extra" is special, it must be a PropertyMap. It is
		// representing a bunch of pre-serialized properties and thus it doesn't
		// need its own codec.
		if opts == "extra" {
			if _, ok := c.bySpecial["extra"]; ok {
				c.problem = me("struct has multiple fields tagged as 'extra'")
				return
			}
			if st.name != "" && st.name != "-" {
				c.problem = me("struct 'extra' field has invalid name %s, expecting `` or `-`", st.name)
				return
			}
			if ft != typeOfPropertyMap {
				c.problem = me("struct 'extra' field has invalid type %s, expecting PropertyMap", ft)
				return
			}
			st.isExtra = true
			c.bySpecial["extra"] = i
			continue
		}

		// Default the property name to the struct field name. Note that we still
		// may end up with an empty name in case this struct embeds a field. Such
		// fields are `f.Anonymous == true` and this case is handled later, when
		// recusing into substructs.
		if st.name == "" {
			if !f.Anonymous {
				st.name = f.Name
			}
		}

		// Only meta keys (like `$kind`) are allowed to be unexported. Skip the
		// rest. Also skip explicitly omitted keys.
		isMeta := strings.HasPrefix(st.name, "$")
		if (!isMeta && !st.exported) || st.name == "-" {
			st.name = "-"
			continue
		}

		// Figure out how to convert this field into a property or a property slice
		// based on its type and `gae:"..."` tag.
		switch {
		// A single value that implements PropertyConverter.
		case reflect.PtrTo(ft).Implements(typeOfPropertyConverter):
			st.convertMethod = convertProp

		// A slice of values, where each implements PropertyConverter.
		case ft.Kind() == reflect.Slice && reflect.PtrTo(ft.Elem()).Implements(typeOfPropertyConverter):
			st.convertMethod = convertProp
			st.isSlice = true

		// A single protobuf message.
		case ft.Implements(typeofProtoMessage):
			st.convertMethod = convertProto
			st.idxSetting = NoIndex
			var err error
			if st.protoOption, err = parseProtoOpts(opts); err != nil {
				c.problem = me("%s %q", err, f.Name)
				return
			}

		// A slice of protobuf messages.
		case ft.Kind() == reflect.Slice && ft.Elem().Implements(typeofProtoMessage):
			// TODO(vadimsh): Should we support them? It almost works.
			c.problem = me("repeated protobuf fields like %q are not supported", f.Name)
			return

		// Something unrecognized we can't recurse into.
		case ft.Kind() == reflect.Interface:
			c.problem = me("field %q has non-concrete interface type %s", f.Name, ft)
			return

		// A single or repeated "local structure property" field.
		case opts == "lsp" || opts == "lsp,noindex":
			st.convertMethod = convertLSP
			// Values must be represented by structs.
			switch {
			case isStruct(ft):
				st.substructCodec = getStructCodecLocked(ft)
			case ft.Kind() == reflect.Slice && isStruct(ft.Elem()):
				st.substructCodec = getStructCodecLocked(ft.Elem())
				st.isSlice = true
			default:
				c.problem = me("lsp fields like %q must be structs or slices of structs", f.Name)
				return
			}

		// A single nested (perhaps anonymous) struct.
		case isStruct(ft):
			st.convertMethod = convertDefault
			st.substructCodec = getStructCodecLocked(ft)

		// A slice of structs.
		case ft.Kind() == reflect.Slice && isStruct(ft.Elem()):
			st.convertMethod = convertDefault
			st.isSlice = true
			st.substructCodec = getStructCodecLocked(ft.Elem())

		// A []byte field. This is a special case, since it is a slice in Go.
		case ft.Kind() == reflect.Slice && ft.Elem().Kind() == reflect.Uint8:
			st.convertMethod = convertDefault

		// A slice of elementary values.
		case ft.Kind() == reflect.Slice:
			st.convertMethod = convertDefault
			st.isSlice = true

		default:
			// A singular elementary value.
			st.convertMethod = convertDefault
		}

		// Verify recursion into the substruct worked.
		if st.substructCodec != nil {
			switch {
			case st.substructCodec.problem == errRecursiveStruct:
				c.problem = me("field %q is recursively defined", f.Name)
				return
			case st.substructCodec.problem != nil:
				c.problem = me("field %q has problem: %s", f.Name, st.substructCodec.problem)
				return
			}
		}

		// Keep track if the struct has slice-valued fields. Checked to forbid
		// nested types that result in `[][]Type` properties not allowed by the
		// datastore.
		c.hasSlice = c.hasSlice || st.isSlice

		// Deal with meta fields.
		if isMeta {
			metaName := st.name[1:]
			if _, ok := c.byMeta[metaName]; ok {
				c.problem = me("meta field %q set multiple times", st.name)
				return
			}
			c.byMeta[metaName] = i
			switch st.convertMethod {
			case convertDefault:
				// If this is some elementary type, parse the default value. This also
				// verifies the field has expected type for a meta (e.g. not a slice).
				mv, err := convertMeta(opts, ft)
				if err != nil {
					c.problem = me("meta field %q has bad type: %s", f.Name, err)
					return
				}
				st.metaVal = mv
			case convertProp, convertProto:
				// Repeated fields aren't allowed as meta fields.
				if st.isSlice {
					c.problem = me("meta field %q is a slice, this is not supported", f.Name)
					return
				}
				// This is a singular field that can be serialized into a property. This
				// is weird, but fine.
			default:
				// All other kinds of fields can't be used as meta properties. This is
				// unnecessary and just adds complexity.
				c.problem = me("meta field %q has unsupported serialization format", f.Name)
				return
			}
			st.name = "-" // not exported as a real property
			continue
		}

		// This must be a valid property name by now.
		if st.name != "" && !validPropertyName(st.name) {
			c.problem = me("struct tag for field %q has invalid property name: %q", f.Name, st.name)
			return
		}

		// Recognize index opt-out. This applies to elementary properties and, if
		// set, to nested structs, i.e. if a nested struct field has `noindex`, its
		// inner properties will be unindexed regardless of their individual
		// indexing settings. It has no effect on protobuf field and fields that
		// implement PropertyConverter.
		if opts == "noindex" || strings.HasSuffix(opts, ",noindex") {
			st.idxSetting = NoIndex
		}

		// Finish dealing with the nested structs (either a singular or repeated).
		if st.convertMethod == convertDefault && st.substructCodec != nil {
			// Check the nested field can be flattened. `[][]Type` is forbidden.
			if st.isSlice && st.substructCodec.hasSlice {
				c.problem = me("flattening nested structs leads to a slice of slices: field %q", f.Name)
				return
			}
			// Flatten the struct-valued field.
			c.hasSlice = c.hasSlice || st.substructCodec.hasSlice
			// Here st.name is empty if `f` is embedded into the parent struct. Its
			// fields will end up nested directly under `f`.
			if st.name != "" {
				st.name += "."
			}
			for relName := range st.substructCodec.byName {
				absName := st.name + relName
				if _, ok := c.byName[absName]; ok {
					c.problem = me("struct tag has repeated property name: %q", absName)
					return
				}
				c.byName[absName] = i
			}
			// We are done with this field.
			continue
		}

		// This field will be represented by a single (perhaps repeated) property.
		// It must have a name.
		if st.name == "" {
			c.problem = me("non-struct embedded field: %q", f.Name)
			return
		}

		// If this is a scalar or a slice of scalars. Check the underlying static
		// type. This is just a type assertion, we don't store the resolved type
		// anywhere.
		if st.convertMethod == convertDefault {
			t := ft
			if st.isSlice {
				t = t.Elem()
			}
			v := UpconvertUnderlyingType(reflect.New(t).Elem().Interface())
			if _, err := PropertyTypeOf(v, false); err != nil {
				c.problem = me("field %q has invalid type: %s", f.Name, ft)
				return
			}
		}

		// We are done with this field.
		if _, ok := c.byName[st.name]; ok {
			c.problem = me("struct tag has repeated property name: %q", st.name)
			return
		}
		c.byName[st.name] = i
	}

	if c.problem == errRecursiveStruct {
		c.problem = nil
	}
	return
}

func parseProtoOpts(opts string) (protoOption, error) {
	switch opts {
	case "zstd", "nocompress", "legacy", "":
		return protoOption(opts), nil
	// This package historically ignored unsupported options. However, it is
	// expected that some variations of compression algo will be added in the
	// future. Therefore, explicitly disallow all other options.
	case "noindex":
		return "", fmt.Errorf("noindex option is redundant and not allowed with proto.Message")
	default:
		return "", fmt.Errorf("unsupported option %q for proto.Message", opts)
	}
}

func convertMeta(val string, t reflect.Type) (any, error) {
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
