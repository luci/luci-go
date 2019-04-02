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

package cli

import (
	"flag"
	"reflect"
	"strings"

	"github.com/golang/protobuf/proto"
	"google.golang.org/genproto/protobuf/field_mask"

	pb "go.chromium.org/luci/buildbucket/proto"
)

type buildFieldFlags struct {
	all        bool
	properties bool
	steps      bool
}

func (f *buildFieldFlags) Register(fs *flag.FlagSet) {
	fs.BoolVar(&f.all, "A", false, "Print build entirely")
	fs.BoolVar(&f.steps, "steps", false, "Print steps")
	fs.BoolVar(&f.properties, "p", false, "Print input/output properties")
}

func (f *buildFieldFlags) FieldMask() *field_mask.FieldMask {
	if f.all {
		ret := &field_mask.FieldMask{}
		for _, p := range proto.GetProperties(reflect.TypeOf(pb.Build{})).Prop {
			if !strings.HasPrefix(p.OrigName, "XXX") {
				ret.Paths = append(ret.Paths, p.OrigName)
			}
		}
		return ret
	}

	ret := &field_mask.FieldMask{
		Paths: []string{
			"builder",
			"create_time",
			"created_by",
			"end_time",
			"id",
			"input.experimental",
			"input.gerrit_changes",
			"input.gitiles_commit",
			"number",
			"start_time",
			"status",
			"status_details",
			"summary_markdown",
			"tags",
			"update_time",
		},
	}

	if f.properties {
		ret.Paths = append(ret.Paths, "input.properties", "output.properties")
	}

	if f.steps {
		ret.Paths = append(ret.Paths, "steps")
	}

	return ret
}
