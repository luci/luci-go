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

	"go.chromium.org/luci/gae/service/datastore/dstypes"
)

const (
	ShouldIndex = dstypes.ShouldIndex
	NoIndex     = dstypes.NoIndex
)

const (
	PTNull        = dstypes.PTNull
	PTInt         = dstypes.PTInt
	PTTime        = dstypes.PTTime
	PTBool        = dstypes.PTBool
	PTBytes       = dstypes.PTBytes
	PTString      = dstypes.PTString
	PTFloat       = dstypes.PTFloat
	PTGeoPoint    = dstypes.PTGeoPoint
	PTKey         = dstypes.PTKey
	PTBlobKey     = dstypes.PTBlobKey
	PTPropertyMap = dstypes.PTPropertyMap
	PTUnknown     = dstypes.PTUnknown
)

type IndexSetting = dstypes.IndexSetting
type PropertyConverter = dstypes.PropertyConverter
type Property = dstypes.Property
type PropertyType = dstypes.PropertyType
type PropertySlice = dstypes.PropertySlice
type PropertyMap = dstypes.PropertyMap
type PropertyData = dstypes.PropertyData
type PropertyLoadSaver = dstypes.PropertyLoadSaver
type MetaGetter = dstypes.MetaGetter
type MetaGetterSetter = dstypes.MetaGetterSetter

func MkProperty(val interface{}) Property {
	return dstypes.MkProperty(val)
}

// MkPropertyNI makes a new Property (with noindex set to true), and returns
// it. If val is an invalid value, this panics (so don't do it). If you want to
// handle the error normally, use SetValue(..., NoIndex) instead.
func MkPropertyNI(val interface{}) Property {
	return dstypes.MkPropertyNI(val)
}

// PropertyTypeOf returns the PT* type of the given Property-compatible
// value v. If checkValid is true, this method will also ensure that time.Time
// and GeoPoint have valid values.
func PropertyTypeOf(v interface{}, checkValid bool) (PropertyType, error) {
	return dstypes.PropertyTypeOf(v, checkValid)
}

// RoundTime rounds a time.Time to microseconds, which is the (undocumented)
// way that the AppEngine SDK stores it.
func RoundTime(t time.Time) time.Time {
	return dstypes.RoundTime(t)
}

// TimeToInt converts a time value to a datastore-appropraite integer value.
//
// This method truncates the time to microseconds and drops the timezone,
// because that's the (undocumented) way that the appengine SDK does it.
func TimeToInt(t time.Time) int64 {
	return dstypes.TimeToInt(t)
}

// IntToTime converts a datastore time integer into its time.Time value.
func IntToTime(v int64) time.Time {
	return dstypes.IntToTime(v)
}

// UpconvertUnderlyingType takes an object o, and attempts to convert it to
// its native datastore-compatible type. e.g. int16 will convert to int64, and
// `type Foo string` will convert to `string`.
func UpconvertUnderlyingType(o interface{}) interface{} {
	return dstypes.UpconvertUnderlyingType(o)
}

func GetMetaDefault(getter MetaGetter, key string, dflt interface{}) interface{} {
	return dstypes.GetMetaDefault(getter, key, dflt)
}
