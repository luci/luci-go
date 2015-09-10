// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prod

import (
	"fmt"
	"reflect"
	"time"

	bs "github.com/luci/gae/service/blobstore"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/datastore/dskey"
	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
)

type typeFilter struct {
	pm ds.PropertyMap
}

var _ datastore.PropertyLoadSaver = &typeFilter{}

func maybeIndexValue(val interface{}) interface{} {
	// It may be the SDK's datastore.indexValue structure (in datastore/load.go).
	//
	// Since this is a private type with no methods, we need to use reflection
	// to get the data out. Ick.
	rv := reflect.ValueOf(val)
	if rv.Kind() == reflect.Struct && rv.Type().String() == "datastore.indexValue" {
		rv = rv.FieldByName("value")
		if rv.IsValid() && rv.Kind() == reflect.Ptr {
			// TODO(riannucci): determine if this is how nil IndexValues are stored.
			// Maybe they're encoded as a PropertyValue with all-nil fields instead?
			if rv.IsNil() {
				return nil
			}
			rv = rv.Elem()
			// we're in protobuf land now.
			if rv.Type().Name() == "PropertyValue" {
				for i := 0; i < rv.NumField(); i++ {
					field := rv.Field(i)
					if field.Kind() == reflect.Ptr {
						if field.IsNil() {
							continue
						}
						field = field.Elem()
						switch field.Kind() {
						case reflect.Int64:
							return field.Int()
						case reflect.String:
							return field.String()
						case reflect.Bool:
							return field.Bool()
						case reflect.Float64:
							return field.Float()
						}
						switch field.Type().Name() {
						case "PropertyValue_PointValue":
							// Lat == X, Lng == Y b/c historical resons.
							return ds.GeoPoint{
								Lat: field.FieldByName("X").Float(),
								Lng: field.FieldByName("Y").Float()}
						case "PropertyValue_ReferenceValue":
							aid := field.FieldByName("App").Elem().String()
							ns := ""
							if nsf := field.FieldByName("NameSpace"); !nsf.IsNil() {
								ns = nsf.Elem().String()
							}
							elems := field.FieldByName("Pathelement")
							toks := make([]ds.KeyTok, elems.Len())
							for i := range toks {
								e := elems.Index(i).Elem()
								toks[i].Kind = e.FieldByName("Type").Elem().String()
								if iid := e.FieldByName("Id"); !iid.IsNil() {
									toks[i].IntID = iid.Elem().Int()
								}
								if sid := e.FieldByName("Name"); !sid.IsNil() {
									toks[i].StringID = sid.Elem().String()
								}
							}
							return dskey.NewToks(aid, ns, toks)
						}
						panic(fmt.Errorf(
							"UNKNOWN datastore.indexValue field type: %s", field.Type()))
					}
					// there's also the `XXX_unrecognized []byte` field, so don't panic
					// here.
				}
				panic(fmt.Errorf("cannot decode datastore.indexValue (no recognized field): %v", val))
			}
			panic(fmt.Errorf("cannot decode datastore.indexValue (wrong inner type): %v", val))
		}
		panic(fmt.Errorf("cannot decode datastore.indexValue: %v", val))
	} else {
		return val
	}
}

func (tf *typeFilter) Load(props []datastore.Property) error {
	tf.pm = make(ds.PropertyMap, len(props))
	for _, p := range props {
		val := p.Value
		switch x := val.(type) {
		case datastore.ByteString:
			val = []byte(x)
		case *datastore.Key:
			val = dsR2F(x)
		case appengine.BlobKey:
			val = bs.Key(x)
		case appengine.GeoPoint:
			val = ds.GeoPoint(x)
		case time.Time:
			// "appengine" layer instantiates with Local timezone.
			val = x.UTC()
		default:
			val = maybeIndexValue(val)
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
			switch prop.Type() {
			case ds.PTBytes:
				v := prop.Value().([]byte)
				if prop.IndexSetting() == ds.ShouldIndex {
					toAdd.Value = datastore.ByteString(v)
				} else {
					toAdd.Value = v
				}
			case ds.PTKey:
				toAdd.Value = dsF2R(prop.Value().(ds.Key))
			case ds.PTBlobKey:
				toAdd.Value = appengine.BlobKey(prop.Value().(bs.Key))
			case ds.PTGeoPoint:
				toAdd.Value = appengine.GeoPoint(prop.Value().(ds.GeoPoint))
			default:
				toAdd.Value = prop.Value()
			}
			props = append(props, toAdd)
		}
	}
	return props, nil
}
