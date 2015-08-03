// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package datastore

import (
	"reflect"
)

// GetPLS resolves o into a PropertyLoadSaver. o must be a pointer to a
// struct of some sort.
func GetPLS(o interface{}) PropertyLoadSaver {
	v := reflect.ValueOf(o)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return &structPLS{c: &structCodec{problem: ErrInvalidEntityType}}
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
