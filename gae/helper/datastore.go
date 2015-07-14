// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// HEAVILY adapted from github.com/golang/appengine/datastore

package helper

import (
	"reflect"

	"infra/gae/libs/gae"
)

// GetPLS resolves o into a gae.DSStructPLS. o must be a pointer to a
// struct of some sort.
func GetPLS(o interface{}) gae.DSPropertyLoadSaver {
	v := reflect.ValueOf(o)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return &structPLS{c: &structCodec{problem: gae.ErrDSInvalidEntityType}}
	}
	v = v.Elem()
	t := v.Type()

	structCodecsMutex.RLock()
	if c, ok := structCodecs[t]; ok {
		structCodecsMutex.RUnlock()
		return &structPLS{v, c}
	}
	structCodecsMutex.RUnlock()

	structCodecsMutex.Lock()
	defer structCodecsMutex.Unlock()
	return &structPLS{v, getStructCodecLocked(t)}
}
