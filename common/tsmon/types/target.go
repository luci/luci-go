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

package types

import (
	"fmt"
	"reflect"

	pb "go.chromium.org/luci/common/tsmon/ts_mon_proto"
)

// A Target knows how to put information about itself in a MetricsData message.
type Target interface {
	PopulateProto(d *pb.MetricsCollection)
	Hash() uint64
	Clone() Target
	Type() TargetType
}

// TargetType represents the type of a Target, which identifies a metric
// with the name.
//
// A metric is identified by (metric.info.name, metric.info.target_type).
type TargetType struct {
	// Name is the name of the TargeType that can be given to --target-type
	// command line option.
	Name string
	// Type is the reflect.Type of the Target struct.
	Type reflect.Type
}

func (tt TargetType) String() string {
	return fmt.Sprintf("%s:%s", tt.Name, tt.Type)
}
