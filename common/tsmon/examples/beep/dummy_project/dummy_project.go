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
	"hash/fnv"
	"reflect"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/tsmon/target"
	pb "go.chromium.org/luci/common/tsmon/ts_mon_proto"
	"go.chromium.org/luci/common/tsmon/types"
)

// Hash returns a uint64 hash of this target.
func (t *DummyProject) Hash() uint64 {
	h := fnv.New64a()
	h.Write([]byte(fmt.Sprintf("%+v", t)))
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
	// * Note for the implementation choice of PopulateProto.
	//
	// types.Target interface requires each Target to implement PopulateProto,
	// and the role of the function is to convert to a given Target instance
	// into MetricsCollection, which is the message format that the tsmon
	// backend supports.
	//
	// There are two ways of implementing the function, and each has pros and
	// cons.
	//
	// (1) Manually list all the target fields.
	// Pros
	// - faster than (2)
	// - the code can look more intuitive, as the mapping between the field
	// name in the monitoring data and the struct field can be easily found.
	//
	// Cons
	// - need to update the list every time the target proto changes.
	// However, please be aware that target proto should be changed carefully.
	// Modifying a target proto requires updating the proto in the monitoring
	// backend, and all the changes must be backward-compatible.
	//
	d.RootLabels = append(
		d.RootLabels,
		target.RootLabel("project", t.Project),
		target.RootLabel("location", t.Location),
		target.RootLabel("is_staging", t.IsStaging),
	)

	// (2) Iterate the proto struct fields and generate RootLabels.
	// Pros
	// - No need to list all the target fields manually.
	// Cons
	// - Slower than (1), as it parses the tag of each field and appends the
	// RootLabel into MetricsCollection iteratively. However, it's questionable
	// whether (2) is meaningfully slower than (1).

	// Target.PopulateProto is invoked once for each Target instance every time
	// tsmon.Flush is invoked. Typically, an application handles a small number
	// of monitoring targets. However, platform software tends to report
	// monitoring data for a large number of different monitoring targets, each
	// represents a client of the platform. If a given Target proto is used to
	// report report monitoring data in platform software, (1) would probably
	// be a better choice, but, the choice of (1) and (2) won't make a much
	// difference, otherwise, because # of monitoring targets is as small as
	// < 10.

	// t.toMetricsProto(d) is an implementation of (2). It can be used to
	// convert any proto-based Target instance to MetricsCollection.
	//
	// t.toMetricsProto(d)
}

func (t *DummyProject) toMetricsProto(d *pb.MetricsCollection) {
	st := t.Type().Type.Elem()
	sv := reflect.Indirect(reflect.ValueOf(t))
	for i := range st.NumField() {
		props := new(proto.Properties)
		tag, ok := st.Field(i).Tag.Lookup("protobuf")
		if !ok {
			continue
		}
		props.Parse(tag)

		var v any
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
	}
}
