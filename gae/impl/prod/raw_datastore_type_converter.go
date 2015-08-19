// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prod

import (
	"time"

	bs "github.com/luci/gae/service/blobstore"
	ds "github.com/luci/gae/service/datastore"
	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
)

type typeFilter struct {
	pm ds.PropertyMap
}

var _ datastore.PropertyLoadSaver = &typeFilter{}

func (tf *typeFilter) Load(props []datastore.Property) error {
	tf.pm = make(ds.PropertyMap, len(props))
	for _, p := range props {
		val := p.Value
		switch x := val.(type) {
		case datastore.ByteString:
			val = ds.ByteString(x)
		case *datastore.Key:
			val = dsR2F(x)
		case appengine.BlobKey:
			val = bs.Key(x)
		case appengine.GeoPoint:
			val = ds.GeoPoint(x)
		case time.Time:
			// "appengine" layer instantiates with Local timezone.
			val = x.UTC()
		}
		prop := ds.Property{}
		is := ds.ShouldIndex
		if p.NoIndex {
			is = ds.NoIndex
		}
		if err := prop.SetValue(val, is); err != nil {
			return err
		}
		tf.pm[p.Name] = append(tf.pm[p.Name], prop)
	}
	return nil
}

func (tf *typeFilter) Save() ([]datastore.Property, error) {
	props := []datastore.Property{}
	for name, propList := range tf.pm {
		if len(name) != 0 && name[0] == '$' {
			continue
		}
		multiple := len(propList) > 1
		for _, prop := range propList {
			toAdd := datastore.Property{
				Name:     name,
				Multiple: multiple,
				NoIndex:  prop.IndexSetting() == ds.NoIndex,
			}
			switch x := prop.Value().(type) {
			case ds.ByteString:
				toAdd.Value = datastore.ByteString(x)
			case ds.Key:
				toAdd.Value = dsF2R(x)
			case bs.Key:
				toAdd.Value = appengine.BlobKey(x)
			case ds.GeoPoint:
				toAdd.Value = appengine.GeoPoint(x)
			default:
				toAdd.Value = x
			}
			props = append(props, toAdd)
		}
	}
	return props, nil
}
