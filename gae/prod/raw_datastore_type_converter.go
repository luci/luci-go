// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prod

import (
	"infra/gae/libs/gae"

	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
)

type typeFilter struct {
	pls gae.DSPropertyLoadSaver
}

var _ datastore.PropertyLoadSaver = &typeFilter{}

func (tf *typeFilter) Load(props []datastore.Property) error {
	pmap := make(gae.DSPropertyMap, len(props))
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
		is := gae.ShouldIndex
		if p.NoIndex {
			is = gae.NoIndex
		}
		if err := prop.SetValue(val, is); err != nil {
			return err
		}
		pmap[p.Name] = append(pmap[p.Name], prop)
	}
	return tf.pls.Load(pmap)
}

func (tf *typeFilter) Save() ([]datastore.Property, error) {
	newProps, err := tf.pls.Save(false)
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
				NoIndex:  prop.IndexSetting() == gae.NoIndex,
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
