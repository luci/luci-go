// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// HEAVILY adapted from github.com/golang/appengine/datastore

package helper

import (
	"fmt"
	"reflect"

	"infra/gae/libs/gae"
)

// GetStructPLS resolves o into a gae.DSStructPLS. o must be a pointer to a
// struct of some sort.
func GetStructPLS(o interface{}) gae.DSStructPLS {
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

// GetPLS resolves o into a gae.DSPropertyLoadSaver. If o implements the
// gae.DSPropertyLoadSaver interface already, it's returned unchanged. Otherwise
// this calls GetStructPLS and returns the result.
func GetPLS(o interface{}) (gae.DSPropertyLoadSaver, error) {
	if pls, ok := o.(gae.DSPropertyLoadSaver); ok {
		return pls, nil
	}
	pls := GetStructPLS(o)
	if err := pls.Problem(); err != nil {
		return nil, err
	}
	return pls, nil
}

// MultiGetPLS resolves os to a []gae.DSPropertyLoadSaver. os must be a slice of
// something. If os is already a []gae.DSPropertyLoadSaver, it's returned
// unchanged. Otherwise this calls GetPLS on each item, and composes the
// resulting slice that way.
func MultiGetPLS(os interface{}) ([]gae.DSPropertyLoadSaver, error) {
	if plss, ok := os.([]gae.DSPropertyLoadSaver); ok {
		return plss, nil
	}

	v := reflect.ValueOf(os)
	if v.Kind() != reflect.Slice {
		return nil, fmt.Errorf("gae/helper: bad type in MultiObjToPLS: %T", os)
	}

	// TODO(riannucci): define a higher-level type DSMultiPropertyLoadSaver
	// to avoid this slice?
	ret := make([]gae.DSPropertyLoadSaver, v.Len())
	for i := 0; i < v.Len(); i++ {
		pls, err := GetPLS(v.Index(i).Interface())
		if err != nil {
			return nil, err
		}
		ret[i] = pls
	}
	return ret, nil
}
