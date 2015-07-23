// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rawdatastore

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"time"

	"github.com/luci/gae/service/blobstore"
)

var (
	minTime = time.Unix(int64(math.MinInt64)/1e6, (int64(math.MinInt64)%1e6)*1e3)
	maxTime = time.Unix(int64(math.MaxInt64)/1e6, (int64(math.MaxInt64)%1e6)*1e3)
)

// IndexSetting indicates whether or not a Property should be indexed by the
// datastore.
type IndexSetting bool

// ShouldIndex is the default, which is why it must assume the zero value,
// even though it's werid :(.
const (
	ShouldIndex IndexSetting = false
	NoIndex     IndexSetting = true
)

func (i IndexSetting) String() string {
	if i {
		return "NoIndex"
	}
	return "ShouldIndex"
}

// Property is a value plus an indicator of whether the value should be
// indexed. Name and Multiple are stored in the PropertyMap object.
type Property struct {
	value        interface{}
	indexSetting IndexSetting
	propType     PropertyType
}

// MkProperty makes a new indexed* Property and returns it. If val is an
// invalid value, this panics (so don't do it). If you want to handle the error
// normally, use SetValue(..., ShouldIndex) instead.
//
// *indexed if val is not an unindexable type like []byte.
func MkProperty(val interface{}) Property {
	ret := Property{}
	if err := ret.SetValue(val, ShouldIndex); err != nil {
		panic(err)
	}
	return ret
}

// MkPropertyNI makes a new Property (with noindex set to true), and returns
// it. If val is an invalid value, this panics (so don't do it). If you want to
// handle the error normally, use SetValue(..., NoIndex) instead.
func MkPropertyNI(val interface{}) Property {
	ret := Property{}
	if err := ret.SetValue(val, NoIndex); err != nil {
		panic(err)
	}
	return ret
}

// PropertyConverter may be implemented by the pointer-to a struct field which
// is serialized by RawDatastore. Its ToProperty will be called on save, and
// it's FromProperty will be called on load (from datastore). The method may
// do arbitrary computation, and if it encounters an error, may return it.  This
// error will be a fatal error (as defined by PropertyLoadSaver) for the
// struct conversion.
//
// Example:
//   type Complex complex
//   func (c *Complex) ToProperty() (ret Property, err error) {
//     // something like:
//     err = ret.SetValue(fmt.Sprint(*c), true)
//     return
//   }
//   func (c *Complex) FromProperty(p Property) (err error) {
//     ... load *c from p ...
//   }
//
//   type MyStruct struct {
//     Complexity []Complex // acts like []complex, but can be serialized to DS
//   }
type PropertyConverter interface {
	// TODO(riannucci): Allow a convertable to return multiple values.  This is
	// eminently doable (as long as the single-slice restriction is kept).  It
	// could also cut down on the amount of reflection necessary when resolving
	// a path in a struct (in the struct loading routine in helper).

	ToProperty() (Property, error)
	FromProperty(Property) error
}

// PropertyType is a single-byte representation of the type of data contained
// in a Property. The specific values of this type information are chosen so
// that the types sort according to the order of types as sorted by the
// datastore.
type PropertyType byte

// These constants are in the order described by
//   https://cloud.google.com/appengine/docs/go/datastore/entities#Go_Value_type_ordering
// with a slight divergence for the Int/Time split.
// NOTE: this enum can only occupy 7 bits, because we use the high bit to encode
// indexed/non-indexed. See typData.WriteBinary.
const (
	PTNull PropertyType = iota
	PTInt

	// PTTime is a slight divergence from the way that datastore natively stores
	// time. In datastore, times and integers actually sort together
	// (apparently?). This is probably insane, and I don't want to add the
	// complexity of field 'meaning' as a sparate concept from the field's 'type'
	// (which is what datastore seems to do, judging from the protobufs). So if
	// you're here because you implemented an app which relies on time.Time and
	// int64 sorting together, then this is why your app acts differently in
	// production. My advice is to NOT DO THAT. If you really want this (and you
	// probably don't), you should take care of the time.Time <-> int64 conversion
	// in your app and just use a property type of int64 (consider using
	// PropertyConverter).
	PTTime

	// PTBoolFalse and True are also a slight divergence, but not a semantic
	// one. IIUC, in datastore 'bool' is actually the type and the value is either
	// 0 or 1 (taking another byte to store). Since we have plenty of space in
	// this type byte, I just merge the value into the type for booleans. If this
	// becomes problematic, consider changing this to just pvBool, and then
	// encoding a 0 or 1 as a byte in the relevant marshalling routines.
	PTBoolFalse
	PTBoolTrue

	PTBytes  // []byte or datastore.ByteString
	PTString // string or string noindex
	PTFloat
	PTGeoPoint
	PTKey
	PTBlobKey

	PTUnknown
)

func (t PropertyType) String() string {
	switch t {
	case PTNull:
		return "PTNull"
	case PTInt:
		return "PTInt"
	case PTTime:
		return "PTTime"
	case PTBoolFalse:
		return "PTBoolFalse"
	case PTBoolTrue:
		return "PTBoolTrue"
	case PTBytes:
		return "PTBytes"
	case PTString:
		return "PTString"
	case PTFloat:
		return "PTFloat"
	case PTGeoPoint:
		return "PTGeoPoint"
	case PTKey:
		return "PTKey"
	case PTBlobKey:
		return "PTBlobKey"
	default:
		return fmt.Sprintf("PTUnknown(%02x)", byte(t))
	}
}

// PropertyTypeOf returns the PT* type of the given Property-compatible
// value v. If checkValid is true, this method will also ensure that time.Time
// and GeoPoint have valid values.
func PropertyTypeOf(v interface{}, checkValid bool) (PropertyType, error) {
	switch x := v.(type) {
	case nil:
		return PTNull, nil
	case int64:
		return PTInt, nil
	case float64:
		return PTFloat, nil
	case bool:
		if x {
			return PTBoolTrue, nil
		}
		return PTBoolFalse, nil
	case []byte, ByteString:
		return PTBytes, nil
	case blobstore.Key:
		return PTBlobKey, nil
	case string:
		return PTString, nil
	case Key:
		// TODO(riannucci): Check key for validity in its own namespace?
		return PTKey, nil
	case time.Time:
		err := error(nil)
		if checkValid && (x.Before(minTime) || x.After(maxTime)) {
			err = errors.New("time value out of range")
		}
		if checkValid && x.Location() != time.UTC {
			err = fmt.Errorf("time value has wrong Location: %s", x.Location())
		}
		return PTTime, err
	case GeoPoint:
		err := error(nil)
		if checkValid && !x.Valid() {
			err = errors.New("invalid GeoPoint value")
		}
		return PTGeoPoint, err
	default:
		return PTUnknown, fmt.Errorf("gae: Property has bad type %T", v)
	}
}

// UpconvertUnderlyingType takes an object o, and attempts to convert it to
// its native datastore-compatible type. e.g. int16 will convert to int64, and
// `type Foo string` will convert to `string`.
func UpconvertUnderlyingType(o interface{}, t reflect.Type) (interface{}, reflect.Type) {
	v := reflect.ValueOf(o)
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		o = v.Int()
		t = typeOfInt64
	case reflect.Bool:
		o = v.Bool()
		t = typeOfBool
	case reflect.String:
		if t != typeOfBSKey {
			o = v.String()
			t = typeOfString
		}
	case reflect.Float32, reflect.Float64:
		o = v.Float()
		t = typeOfFloat64
	case reflect.Slice:
		if t != typeOfByteString && t.Elem().Kind() == reflect.Uint8 {
			o = v.Bytes()
			t = typeOfByteSlice
		}
	case reflect.Struct:
		if t == typeOfTime {
			// time in a Property can only hold microseconds
			o = v.Interface().(time.Time).Round(time.Microsecond)
		}
	}
	return o, t
}

// Value returns the current value held by this property. It's guaranteed to
// be a valid value type (i.e. `p.SetValue(p.Value(), true)` will never return
// an error).
func (p Property) Value() interface{} { return p.value }

// IndexSetting says weather or not the datastore should create indicies for
// this value.
func (p Property) IndexSetting() IndexSetting { return p.indexSetting }

// Type is the PT* type of the data contained in Value().
func (p Property) Type() PropertyType { return p.propType }

// SetValue sets the Value field of a Property, and ensures that its value
// conforms to the permissible types. That way, you're guaranteed that if you
// have a Property, its value is valid.
//
// value is the property value. The valid types are:
//	- int64
//	- bool
//	- string
//	- float64
//	- ByteString
//	- Key
//	- time.Time
//	- blobstore.Key
//	- GeoPoint
//	- []byte (up to 1 megabyte in length)
// This set is smaller than the set of valid struct field types that the
// datastore can load and save. A Property Value cannot be a slice (apart
// from []byte); use multiple Properties instead. Also, a Value's type
// must be explicitly on the list above; it is not sufficient for the
// underlying type to be on that list. For example, a Value of "type
// myInt64 int64" is invalid. Smaller-width integers and floats are also
// invalid. Again, this is more restrictive than the set of valid struct
// field types.
//
// A value may also be the nil interface value; this is equivalent to
// Python's None but not directly representable by a Go struct. Loading
// a nil-valued property into a struct will set that field to the zero
// value.
func (p *Property) SetValue(value interface{}, is IndexSetting) (err error) {
	t := reflect.Type(nil)
	pt := PTNull
	if value != nil {
		t = reflect.TypeOf(value)
		value, t = UpconvertUnderlyingType(value, t)
		if pt, err = PropertyTypeOf(value, true); err != nil {
			return
		}
	}
	p.propType = pt
	p.value = value
	p.indexSetting = is
	if t == typeOfByteSlice {
		p.indexSetting = NoIndex
	}
	return
}

// PropertyLoadSaver may be implemented by a user type, and RawDatastore will
// use this interface to serialize the type instead of trying to automatically
// create a serialization codec for it with helper.GetPLS.
type PropertyLoadSaver interface {
	// Load takes the values from the given map and attempts to save them into
	// the underlying object (usually a struct or a PropertyMap). If a fatal
	// error occurs, it's returned via error. If non-fatal conversion errors
	// occur, error will be a MultiError containing one or more ErrFieldMismatch
	// objects.
	Load(PropertyMap) error

	// Save returns the current property as a PropertyMap. if withMeta is true,
	// then the PropertyMap contains all the metadata (e.g. '$meta' fields)
	// which was held by this PropertyLoadSaver.
	Save(withMeta bool) (PropertyMap, error)

	// GetMeta will get information about the field which has the struct tag in
	// the form of `gae:"$<key>[,<value>]?"`.
	//
	// string and int64 fields will return the <value> in the struct tag,
	// converted to the appropriate type, if the field has the zero value.
	//
	// Example:
	//   type MyStruct struct {
	//     CoolField int64 `gae:"$id,1"`
	//   }
	//   val, err := helper.GetPLS(&MyStruct{}).GetMeta("id")
	//   // val == 1
	//   // err == nil
	//
	//   val, err := helper.GetPLS(&MyStruct{10}).GetMeta("id")
	//   // val == 10
	//   // err == nil
	GetMeta(key string) (interface{}, error)

	// SetMeta allows you to set the current value of the meta-keyed field.
	SetMeta(key string, val interface{}) error

	// Problem indicates that this PLS has a fatal problem. Usually this is
	// set when the underlying struct has recursion, invalid field types, nested
	// slices, etc.
	Problem() error
}

// PropertyMap represents the contents of a datastore entity in a generic way.
// It maps from property name to a list of property values which correspond to
// that property name. It is the spiritual successor to PropertyList from the
// original SDK.
//
// PropertyMap may contain "meta" values, which are keyed with a '$' prefix.
// Technically the datastore allows arbitrary property names, but all of the
// SDKs go out of their way to try to make all property names valid programming
// language tokens. Special values must correspond to a single Property...
// corresponding to 0 is equivalent to unset, and corresponding to >1 is an
// error. So:
//
//   {
//     "$id": {MkProperty(1)}, // GetProperty("id") -> 1, nil
//     "$foo": {}, // GetProperty("foo") -> nil, ErrMetaFieldUnset
//     // GetProperty("bar") -> nil, ErrMetaFieldUnset
//     "$meep": {
//       MkProperty("hi"),
//       MkProperty("there")}, // GetProperty("meep") -> nil, error!
//   }
//
// Additionally, Save returns a copy of the map with the meta keys omitted (e.g.
// these keys are not going to be serialized to the datastore).
type PropertyMap map[string][]Property

var _ PropertyLoadSaver = PropertyMap(nil)

// Load implements PropertyLoadSaver.Load
func (pm PropertyMap) Load(props PropertyMap) error {
	for k, v := range props {
		pm[k] = append(pm[k], v...)
	}
	return nil
}

// Save implements PropertyLoadSaver.Save by returning a copy of the
// current map data.
func (pm PropertyMap) Save(withMeta bool) (PropertyMap, error) {
	if len(pm) == 0 {
		return PropertyMap{}, nil
	}
	ret := make(PropertyMap, len(pm))
	for k, v := range pm {
		if withMeta || len(k) == 0 || k[0] != '$' {
			ret[k] = append(ret[k], v...)
		}
	}
	return ret, nil
}

// GetMeta implements PropertyLoadSaver.GetMeta, and returns the current value
// associated with the metadata key. It may return ErrMetaFieldUnset if the
// key doesn't exist.
func (pm PropertyMap) GetMeta(key string) (interface{}, error) {
	v, ok := pm["$"+key]
	if !ok || len(v) == 0 {
		return nil, ErrMetaFieldUnset
	}
	if len(v) > 1 {
		return nil, errors.New("gae: too many values for Meta key")
	}
	return v[0].Value(), nil
}

// SetMeta implements PropertyLoadSaver.SetMeta. It will only return an error
// if `val` has an invalid type (e.g. not one supported by Property).
func (pm PropertyMap) SetMeta(key string, val interface{}) error {
	prop := Property{}
	if err := prop.SetValue(val, NoIndex); err != nil {
		return err
	}
	pm["$"+key] = []Property{prop}
	return nil
}

// Problem implements PropertyLoadSaver.Problem. It ALWAYS returns nil.
func (pm PropertyMap) Problem() error {
	return nil
}
