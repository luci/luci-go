// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package datastore

import (
	"reflect"
	"time"

	"github.com/luci/gae/service/blobstore"
)

var (
	typeOfBool              = reflect.TypeOf(true)
	typeOfBSKey             = reflect.TypeOf(blobstore.Key(""))
	typeOfCursorCB          = reflect.TypeOf(CursorCB(nil))
	typeOfGeoPoint          = reflect.TypeOf(GeoPoint{})
	typeOfInt64             = reflect.TypeOf(int64(0))
	typeOfKey               = reflect.TypeOf((*Key)(nil))
	typeOfPropertyConverter = reflect.TypeOf((*PropertyConverter)(nil)).Elem()
	typeOfPropertyLoadSaver = reflect.TypeOf((*PropertyLoadSaver)(nil)).Elem()
	typeOfString            = reflect.TypeOf("")
	typeOfTime              = reflect.TypeOf(time.Time{})
	typeOfToggle            = reflect.TypeOf(Auto)
)
