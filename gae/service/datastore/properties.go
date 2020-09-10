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

// Type aliases for the file dstypes/properties.go

package datastore

import (
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

var MkProperty = dstypes.MkProperty
var MkPropertyNI = dstypes.MkPropertyNI
var PropertyTypeOf = dstypes.PropertyTypeOf
var RoundTime = dstypes.RoundTime
var TimeToInt = dstypes.TimeToInt
var IntToTime = dstypes.IntToTime
var UpconvertUnderlyingType = dstypes.UpconvertUnderlyingType
var GetMetaDefault = dstypes.GetMetaDefault
