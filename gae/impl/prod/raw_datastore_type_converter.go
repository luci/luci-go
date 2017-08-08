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

package prod

import (
	"fmt"
	"reflect"
	"time"

	bs "go.chromium.org/gae/service/blobstore"
	ds "go.chromium.org/gae/service/datastore"
	"golang.org/x/net/context"
	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
)

type typeFilter struct {
	ctx context.Context

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
							return ds.MkKeyContext(aid, ns).NewKeyToks(toks)
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

func dsR2FProp(in datastore.Property) (ds.Property, error) {
	val := in.Value
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
		if x.IsZero() {
			val = time.Time{}
		} else {
			val = x.UTC()
		}
	default:
		val = maybeIndexValue(val)
	}
	ret := ds.Property{}
	is := ds.ShouldIndex
	if in.NoIndex {
		is = ds.NoIndex
	}
	err := ret.SetValue(val, is)
	return ret, err
}

func dsF2RProp(ctx context.Context, in ds.Property) (datastore.Property, error) {
	err := error(nil)
	ret := datastore.Property{
		NoIndex: in.IndexSetting() == ds.NoIndex,
	}
	switch in.Type() {
	case ds.PTBytes:
		v := in.Value().([]byte)
		if in.IndexSetting() == ds.ShouldIndex {
			ret.Value = datastore.ByteString(v)
		} else {
			ret.Value = v
		}
	case ds.PTKey:
		ret.Value, err = dsF2R(ctx, in.Value().(*ds.Key))
	case ds.PTBlobKey:
		ret.Value = appengine.BlobKey(in.Value().(bs.Key))
	case ds.PTGeoPoint:
		ret.Value = appengine.GeoPoint(in.Value().(ds.GeoPoint))
	default:
		ret.Value = in.Value()
	}
	return ret, err
}

func (tf *typeFilter) Load(props []datastore.Property) error {
	tf.pm = make(ds.PropertyMap, len(props))
	for _, p := range props {
		prop, err := dsR2FProp(p)
		if err != nil {
			return err
		}

		pdata := tf.pm[p.Name]
		if p.Multiple {
			var pslice ds.PropertySlice
			if pdata != nil {
				var ok bool
				if pslice, ok = pdata.(ds.PropertySlice); !ok {
					return fmt.Errorf("mixed Multiple/non-Multiple properties for %q", p.Name)
				}
			}
			tf.pm[p.Name] = append(pslice, prop)
		} else {
			if pdata != nil {
				return fmt.Errorf("multiple properties for non-Multiple %q", p.Name)
			}
			tf.pm[p.Name] = prop
		}
	}
	return nil
}

func (tf *typeFilter) Save() ([]datastore.Property, error) {
	props := []datastore.Property{}
	for name, pdata := range tf.pm {
		if len(name) != 0 && name[0] == '$' {
			continue
		}

		var (
			pslice   ds.PropertySlice
			multiple bool
		)
		switch t := pdata.(type) {
		case ds.Property:
			pslice = ds.PropertySlice{t}
		case ds.PropertySlice:
			pslice, multiple = t, true
		default:
			return nil, fmt.Errorf("unknown PropertyData type %T", t)
		}

		for _, prop := range pslice {
			toAdd, err := dsF2RProp(tf.ctx, prop)
			if err != nil {
				return nil, err
			}
			toAdd.Name = name
			toAdd.Multiple = multiple
			props = append(props, toAdd)
		}
	}
	return props, nil
}
