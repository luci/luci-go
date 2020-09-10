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
	"time"

	"go.chromium.org/luci/gae/service/datastore/types"
)

const (
	ShouldIndex = types.ShouldIndex
	NoIndex     = types.NoIndex
)

const (
	PTNull        = types.PTNull
	PTInt         = types.PTInt
	PTTime        = types.PTTime
	PTBool        = types.PTBool
	PTBytes       = types.PTBytes
	PTString      = types.PTString
	PTFloat       = types.PTFloat
	PTGeoPoint    = types.PTGeoPoint
	PTKey         = types.PTKey
	PTBlobKey     = types.PTBlobKey
	PTPropertyMap = types.PTPropertyMap
	PTUnknown     = types.PTUnknown
)

type IndexSetting = types.IndexSetting
type PropertyConverter = types.PropertyConverter
type Property = types.Property
type PropertyType = types.PropertyType
type PropertySlice = types.PropertySlice
type PropertyMap = types.PropertyMap
type PropertyData = types.PropertyData
type PropertyLoadSaver = types.PropertyLoadSaver
type MetaGetter = types.MetaGetter
type MetaGetterSetter = types.MetaGetterSetter

func MkProperty(val interface{}) Property {
	return types.MkProperty(val)
}

// MkPropertyNI makes a new Property (with noindex set to true), and returns
// it. If val is an invalid value, this panics (so don't do it). If you want to
// handle the error normally, use SetValue(..., NoIndex) instead.
func MkPropertyNI(val interface{}) Property {
	return types.MkPropertyNI(val)
}

// PropertyTypeOf returns the PT* type of the given Property-compatible
// value v. If checkValid is true, this method will also ensure that time.Time
// and GeoPoint have valid values.
func PropertyTypeOf(v interface{}, checkValid bool) (PropertyType, error) {
	return types.PropertyTypeOf(v, checkValid)
}

// RoundTime rounds a time.Time to microseconds, which is the (undocumented)
// way that the AppEngine SDK stores it.
func RoundTime(t time.Time) time.Time {
	return types.RoundTime(t)
}

// TimeToInt converts a time value to a datastore-appropraite integer value.
//
// This method truncates the time to microseconds and drops the timezone,
// because that's the (undocumented) way that the appengine SDK does it.
func TimeToInt(t time.Time) int64 {
	return types.TimeToInt(t)
}

// IntToTime converts a datastore time integer into its time.Time value.
func IntToTime(v int64) time.Time {
	return types.IntToTime(v)
}

// UpconvertUnderlyingType takes an object o, and attempts to convert it to
// its native datastore-compatible type. e.g. int16 will convert to int64, and
// `type Foo string` will convert to `string`.
func UpconvertUnderlyingType(o interface{}) interface{} {
	return types.UpconvertUnderlyingType(o)
}

func GetMetaDefault(getter MetaGetter, key string, dflt interface{}) interface{} {
	return types.GetMetaDefault(getter, key, dflt)
}
