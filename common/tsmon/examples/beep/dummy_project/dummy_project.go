// Copyright 2019 The LUCI Authors.
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

// dummy_project implements a monitoring target interface for DummyProject.
package dummy_project

import (
	"fmt"
	"reflect"
	"hash/fnv"

	"github.com/golang/protobuf/proto"

	pb "go.chromium.org/luci/common/tsmon/ts_mon_proto"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/common/tsmon/target"
)


// Hash returns a uint64 hash of this target.
func (t *DummyProject) Hash() uint64 {
	h := fnv.New64a()
	bytes, err := proto.Marshal((*DummyProject)(t))
	if err != nil {
		// bad code.
		panic(err)
	}
	h.Write(bytes)
	return h.Sum64()
}

// Type returns the TargetType of DummyProject.
func (t *DummyProject) Type() types.TargetType {
	pname := proto.MessageName((*DummyProject)(t))
	if pname == "" {
		panic("a unregistered proto target.")
	}
	return types.TargetType{
		Name: proto.MessageName((*DummyProject)(t)),
		Type: reflect.TypeOf(t),
	}
}

// Clone returns a copy of this object.
func (t *DummyProject) Clone() types.Target {
	clone := *t
	return &clone
}

// PopulateProto implements Target.
func (t *DummyProject) PopulateProto(d *pb.MetricsCollection) {
	d.RootLabels = append(
		d.RootLabels,
		target.RootLabel("project", t.Project),
		target.RootLabel("location", t.Location),
		target.RootLabel("is_staging", t.IsStaging),
	)

	/* The below logic can be used instead of the above logic to avoid having
	*  to hard-code the target field names. However, please prefer to implement
	*  in the above way, if reasonable, to avoid unnecessary tag parsing and
	*  field value resolving.

	st := t.Type().Type.Elem()
	sv := reflect.Indirect(reflect.ValueOf(t))
	for i := 0; i < st.NumField(); i++ {
		props := new(proto.Properties)
		tag, ok := st.Field(i).Tag.Lookup("protobuf")
		if !ok {
			continue
		}
		props.Parse(tag)

		var v interface{}
		switch fv := sv.Field(i); fv.Kind() {
		case reflect.Int64:
			v = fv.Int()
		case reflect.String:
			v = fv.String()
		case reflect.Bool:
			v = fv.Bool()
		default:
			panic(fmt.Sprintf("Unsupported target-field type %q", fv.Kind()))
		}
		d.RootLabels = append(
			d.RootLabels, target.RootLabel(props.OrigName, v))
	}*/
}
