// Copyright 2023 The LUCI Authors.
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

package datastore

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	pb "go.chromium.org/luci/gae/service/datastore/internal/protos/datastore"
)

// loadLegacyLSP deserializes a "local structure property" into a property map.
//
// It supports only basic set of property types (e.g. no geo-points or keys).
func loadLegacyLSP(blob []byte) (PropertyMap, error) {
	// Key and EntityGroup are marked `required` in the proto, but they aren't
	// actually populate by Python code.
	ent := pb.EntityProto{}
	if err := (proto.UnmarshalOptions{AllowPartial: true}).Unmarshal(blob, &ent); err != nil {
		return nil, err
	}

	switch {
	case ent.Key != nil:
		return nil, fmt.Errorf("unexpectedly populated `key` in %s", &ent)
	case ent.EntityGroup != nil:
		return nil, fmt.Errorf("unexpectedly populated `entity_group` in %s", &ent)
	case ent.Owner != nil:
		return nil, fmt.Errorf("unexpectedly populated `owner` in %s", &ent)
	case ent.Kind != nil:
		return nil, fmt.Errorf("unexpectedly populated `kind` in %s", &ent)
	case ent.KindUri != nil:
		return nil, fmt.Errorf("unexpectedly populated `kind_uri` in %s", &ent)
	}

	pmap := make(PropertyMap, len(ent.Property)+len(ent.RawProperty))

	addOne := func(p *pb.Property) error {
		name, val, err := decodeLSPProp(p)
		if err != nil {
			return err
		}
		switch cur := pmap[name].(type) {
		case PropertySlice:
			pmap[name] = append(cur, val)
		case Property:
			pmap[name] = PropertySlice{cur, val}
		case nil:
			if p.GetMultiple() {
				pmap[name] = PropertySlice{val}
			} else {
				pmap[name] = val
			}
		default:
			panic("impossible")
		}
		return nil
	}

	for _, prop := range ent.Property {
		if err := addOne(prop); err != nil {
			return nil, err
		}
	}
	for _, prop := range ent.RawProperty {
		if err := addOne(prop); err != nil {
			return nil, err
		}
	}
	return pmap, nil
}

func decodeLSPProp(p *pb.Property) (name string, prop Property, err error) {
	if name = p.GetName(); name == "" {
		err = fmt.Errorf("there's a property without a name")
		return
	}

	v := p.Value
	if v == nil {
		err = fmt.Errorf("property %q has no value", name)
		return
	}

	switch {
	case v.Int64Value != nil:
		prop = MkPropertyNI(*v.Int64Value)
	case v.BooleanValue != nil:
		prop = MkPropertyNI(*v.BooleanValue)
	case v.StringValue != nil && propertyIsBytes(p.GetMeaning()):
		prop = MkPropertyNI([]byte(*v.StringValue))
	case v.StringValue != nil:
		prop = MkPropertyNI(*v.StringValue)
	case v.DoubleValue != nil:
		prop = MkPropertyNI(*v.DoubleValue)
	case v.Pointvalue != nil || v.Referencevalue != nil || v.Uservalue != nil:
		err = fmt.Errorf("unsupported LSP property: %s", p)
	default:
		prop = MkPropertyNI(nil)
	}

	return
}

func propertyIsBytes(m pb.Property_Meaning) bool {
	return m == pb.Property_BLOB || m == pb.Property_BYTESTRING || m == pb.Property_ENTITY_PROTO
}
