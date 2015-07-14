// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gae

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"time"
)

var (
	// ErrDSSpecialFieldUnset is returned from DSStructPLS.{Get,Set}Special
	// implementations when the specified special key isn't set on the struct at
	// all.
	ErrDSSpecialFieldUnset = fmt.Errorf("gae: special field unset")
)

var (
	typeOfBSKey        = reflect.TypeOf(BSKey(""))
	typeOfBool         = reflect.TypeOf(false)
	typeOfByteSlice    = reflect.TypeOf([]byte(nil))
	typeOfDSByteString = reflect.TypeOf(DSByteString(nil))
	typeOfDSGeoPoint   = reflect.TypeOf(DSGeoPoint{})
	typeOfDSKey        = reflect.TypeOf((*DSKey)(nil)).Elem()
	typeOfFloat64      = reflect.TypeOf(float64(0))
	typeOfInt64        = reflect.TypeOf(int64(0))
	typeOfString       = reflect.TypeOf("")
	typeOfTime         = reflect.TypeOf(time.Time{})
)

var (
	minTime = time.Unix(int64(math.MinInt64)/1e6, (int64(math.MinInt64)%1e6)*1e3)
	maxTime = time.Unix(int64(math.MaxInt64)/1e6, (int64(math.MaxInt64)%1e6)*1e3)
)

// DSProperty is a value plus an indicator of whether the value should be
// indexed. Name and Multiple are stored in the DSPropertyMap object.
type DSProperty struct {
	value    interface{}
	noIndex  bool
	propType DSPropertyType
}

// DSPropertyConverter may be implemented by the pointer-to a struct field which
// is serialized by RawDatastore. Its ToDSProperty will be called on save, and
// it's FromDSProperty will be called on load (from datastore). The method may
// do arbitrary computation, and if it encounters an error, may return it.  This
// error will be a fatal error (as defined by DSPropertyLoadSaver) for the
// struct conversion.
//
// Example:
//   type Complex complex
//   func (c *Complex) ToDSProperty() (ret DSProperty, err error) {
//     // something like:
//     err = ret.SetValue(fmt.Sprint(*c), true)
//     return
//   }
//   func (c *Complex) FromDSProperty(p DSProperty) (err error) {
//     ... load *c from p ...
//   }
//
//   type MyStruct struct {
//     Complexity []Complex // acts like []complex, but can be serialized to DS
//   }
type DSPropertyConverter interface {
	// TODO(riannucci): Allow a convertable to return multiple values.  This is
	// eminently doable (as long as the single-slice restriction is kept).  It
	// could also cut down on the amount of reflection necessary when resolving
	// a path in a struct (in the struct loading routine in helper).

	ToDSProperty() (DSProperty, error)
	FromDSProperty(DSProperty) error
}

// DSPropertyType is a single-byte representation of the type of data contained
// in a DSProperty. The specific values of this type information are chosen so
// that the types sort according to the order of types as sorted by the
// datastore.
type DSPropertyType byte

// These constants are in the order described by
//   https://cloud.google.com/appengine/docs/go/datastore/entities#Go_Value_type_ordering
// with a slight divergence for the Int/Time split.
// NOTE: this enum can only occupy 7 bits, because we use the high bit to encode
// indexed/non-indexed. See typData.WriteBinary.
const (
	DSPTNull DSPropertyType = iota
	DSPTInt

	// DSPTTime is a slight divergence from the way that datastore natively stores
	// time. In datastore, times and integers actually sort together
	// (apparently?). This is probably insane, and I don't want to add the
	// complexity of field 'meaning' as a sparate concept from the field's 'type'
	// (which is what datastore seems to do, judging from the protobufs). So if
	// you're here because you implemented an app which relies on time.Time and
	// int64 sorting together, then this is why your app acts differently in
	// production. My advice is to NOT DO THAT. If you really want this (and you
	// probably don't), you should take care of the time.Time <-> int64 conversion
	// in your app and just use a property type of int64 (consider using
	// DSPropertyConverter).
	DSPTTime

	// DSPTBoolFalse and True are also a slight divergence, but not a semantic
	// one. IIUC, in datastore 'bool' is actually the type and the value is either
	// 0 or 1 (taking another byte to store). Since we have plenty of space in
	// this type byte, I just merge the value into the type for booleans. If this
	// becomes problematic, consider changing this to just pvBool, and then
	// encoding a 0 or 1 as a byte in the relevant marshalling routines.
	DSPTBoolFalse
	DSPTBoolTrue

	DSPTBytes  // []byte or datastore.ByteString
	DSPTString // string or string noindex
	DSPTFloat
	DSPTGeoPoint
	DSPTKey
	DSPTBlobKey

	DSPTUnknown
)

func (t DSPropertyType) String() string {
	switch t {
	case DSPTNull:
		return "DSPTNull"
	case DSPTInt:
		return "DSPTInt"
	case DSPTTime:
		return "DSPTTime"
	case DSPTBoolFalse:
		return "DSPTBoolFalse"
	case DSPTBoolTrue:
		return "DSPTBoolTrue"
	case DSPTBytes:
		return "DSPTBytes"
	case DSPTString:
		return "DSPTString"
	case DSPTFloat:
		return "DSPTFloat"
	case DSPTGeoPoint:
		return "DSPTGeoPoint"
	case DSPTKey:
		return "DSPTKey"
	case DSPTBlobKey:
		return "DSPTBlobKey"
	default:
		return "DSPTUnknown"
	}
}

// DSPropertyTypeOf returns the DSPT* type of the given DSProperty-compatible
// value v. If checkValid is true, this method will also ensure that time.Time
// and DSGeoPoint have valid values.
func DSPropertyTypeOf(v interface{}, checkValid bool) (DSPropertyType, error) {
	switch x := v.(type) {
	case nil:
		return DSPTNull, nil
	case int64:
		return DSPTInt, nil
	case float64:
		return DSPTFloat, nil
	case bool:
		if x {
			return DSPTBoolTrue, nil
		}
		return DSPTBoolFalse, nil
	case []byte, DSByteString:
		return DSPTBytes, nil
	case BSKey:
		return DSPTBlobKey, nil
	case string:
		return DSPTString, nil
	case DSKey:
		// TODO(riannucci): Check key for validity in its own namespace?
		return DSPTKey, nil
	case time.Time:
		err := error(nil)
		if checkValid && (x.Before(minTime) || x.After(maxTime)) {
			err = errors.New("time value out of range")
		}
		if checkValid && x.Location() != time.UTC {
			err = fmt.Errorf("time value has wrong Location: %s", x.Location())
		}
		return DSPTTime, err
	case DSGeoPoint:
		err := error(nil)
		if checkValid && !x.Valid() {
			err = errors.New("invalid GeoPoint value")
		}
		return DSPTGeoPoint, err
	default:
		return DSPTUnknown, fmt.Errorf("gae: DSProperty has bad type %T", v)
	}
}

// DSUpconvertUnderlyingType takes an object o, and attempts to convert it to
// its native datastore-compatible type. e.g. int16 will convert to int64, and
// `type Foo string` will convert to `string`.
func DSUpconvertUnderlyingType(o interface{}, t reflect.Type) (interface{}, reflect.Type) {
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
		if t != typeOfDSByteString && t.Elem().Kind() == reflect.Uint8 {
			o = v.Bytes()
			t = typeOfByteSlice
		}
	case reflect.Struct:
		if t == typeOfTime {
			// time in a DSProperty can only hold microseconds
			o = v.Interface().(time.Time).Round(time.Microsecond)
		}
	}
	return o, t
}

// Value returns the current value held by this property. It's guaranteed to
// be a valid value type (i.e. `p.SetValue(p.Value(), true)` will never return
// an error).
func (p DSProperty) Value() interface{} { return p.value }

// NoIndex says weather or not the datastore should create indicies for this
// value.
func (p DSProperty) NoIndex() bool { return p.noIndex }

// Type is the DSPT* type of the data contained in Value().
func (p DSProperty) Type() DSPropertyType { return p.propType }

// SetValue sets the Value field of a DSProperty, and ensures that its value
// conforms to the permissible types. That way, you're guaranteed that if you
// have a DSProperty, its value is valid.
//
// value is the property value. The valid types are:
//	- int64
//	- bool
//	- string
//	- float64
//	- DSByteString
//	- DSKey
//	- time.Time
//	- BSKey
//	- DSGeoPoint
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
//
// noIndex == false will attempt to set NoIndex to false as well. However, if
// value is an unindexable type, noIndex will be coerced to true automatically.
func (p *DSProperty) SetValue(value interface{}, noIndex bool) (err error) {
	t := reflect.Type(nil)
	pt := DSPTNull
	if value != nil {
		t = reflect.TypeOf(value)
		value, t = DSUpconvertUnderlyingType(value, t)
		if pt, err = DSPropertyTypeOf(value, true); err != nil {
			return
		}
	}
	p.propType = pt
	p.value = value
	p.noIndex = noIndex || t == typeOfByteSlice
	return
}

// DSPropertyLoadSaver may be implemented by a user type, and RawDatastore will
// use this interface to serialize the type instead of trying to automatically
// create a serialization codec for it with helper.GetStructPLS.
type DSPropertyLoadSaver interface {
	Load(DSPropertyMap) (convFailures []string, fatal error)
	Save() (DSPropertyMap, error)
}

// DSStructPLS is a DSPropertyLoadSaver, but with some bonus features which only
// apply to user structs (instead of raw DSPropertyMap's).
type DSStructPLS interface {
	DSPropertyLoadSaver

	// GetSpecial will get information about the struct field which has the
	// struct tag in the form of `gae:"$<key>"`.
	//
	// Example:
	//   type MyStruct struct {
	//     CoolField int `gae:"$id,cool"`
	//   }
	//   val, current, err := helper.GetStructPLS(&MyStruct{10}).GetSpecial("id")
	//   // val == "cool"
	//   // current == 10
	//   // err == nil
	GetSpecial(key string) (val string, current interface{}, err error)

	// SetSpecial allows you to set the current value of the special-keyed field.
	SetSpecial(key string, val interface{}) error

	// Problem indicates that this StructPLS has a fatal problem. Usually this is
	// set when the underlying struct has recursion, invalid field types, nested
	// slices, etc.
	Problem() error
}

// DSPropertyMap represents the contents of a datastore entity in a generic way.
// It maps from property name to a list of property values which correspond to
// that property name. It is the spiritual successor to PropertyList from the
// original SDK.
type DSPropertyMap map[string][]DSProperty

var _ DSPropertyLoadSaver = (*DSPropertyMap)(nil)

// Load implements DSPropertyLoadSaver.Load
func (pm *DSPropertyMap) Load(props DSPropertyMap) (convErr []string, err error) {
	if pm == nil {
		return nil, errors.New("gae: nil DSPropertyMap")
	}
	if *pm == nil {
		*pm = make(DSPropertyMap, len(props))
	}
	for k, v := range props {
		(*pm)[k] = append((*pm)[k], v...)
	}
	return nil, nil
}

// Save implements DSPropertyLoadSaver.Save
func (pm *DSPropertyMap) Save() (DSPropertyMap, error) {
	if pm == nil {
		return nil, errors.New("gae: nil DSPropertyMap")
	}
	return *pm, nil
}
