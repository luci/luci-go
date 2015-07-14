// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prod

import (
	"errors"

	"infra/gae/libs/gae"

	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
)

type typeFilter struct {
	dps gae.DSPropertyLoadSaver
}

var _ datastore.PropertyLoadSaver = &typeFilter{}

func (tf *typeFilter) Load(props []datastore.Property) (err error) {
	newProps := map[string][]gae.DSProperty{}
	for _, p := range props {
		val := p.Value
		switch x := val.(type) {
		case datastore.ByteString:
			val = gae.DSByteString(x)
		case *datastore.Key:
			val = dsR2F(x)
		case appengine.BlobKey:
			val = gae.BSKey(x)
		case appengine.GeoPoint:
			val = gae.DSGeoPoint(x)
		}
		prop := gae.DSProperty{}
		if err = prop.SetValue(val, p.NoIndex); err != nil {
			return err
		}
		newProps[p.Name] = append(newProps[p.Name], prop)
	}
	convFailures, err := tf.dps.Load(newProps)
	if err == nil && len(convFailures) > 0 {
		me := make(gae.MultiError, len(convFailures))
		for i, f := range convFailures {
			me[i] = errors.New(f)
		}
		err = me
	}
	return
}

func (tf *typeFilter) Save() ([]datastore.Property, error) {
	newProps, err := tf.dps.Save()
	if err != nil {
		return nil, err
	}

	props := []datastore.Property{}
	for name, propList := range newProps {
		multiple := len(propList) > 1
		for _, prop := range propList {
			toAdd := datastore.Property{
				Name:     name,
				Multiple: multiple,
				NoIndex:  prop.NoIndex(),
			}
			switch x := prop.Value().(type) {
			case gae.DSByteString:
				toAdd.Value = datastore.ByteString(x)
			case gae.DSKey:
				toAdd.Value = dsF2R(x)
			case gae.BSKey:
				toAdd.Value = appengine.BlobKey(x)
			case gae.DSGeoPoint:
				toAdd.Value = appengine.GeoPoint(x)
			default:
				toAdd.Value = x
			}
			props = append(props, toAdd)
		}
	}

	return props, nil
}
