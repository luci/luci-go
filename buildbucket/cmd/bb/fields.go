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

package main

import (
	"flag"
	"reflect"
	"strings"

	"github.com/golang/protobuf/proto"
	"google.golang.org/genproto/protobuf/field_mask"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

type buildFieldFlags struct {
	all              bool
	tags             bool
	inputProperties  bool
	outputProperties bool
	steps            bool
}

func (f *buildFieldFlags) Register(fs *flag.FlagSet) {
	fs.BoolVar(&f.all, "A", false, "output build entirely")
	fs.BoolVar(&f.tags, "tags", false, "output tags")
	fs.BoolVar(&f.steps, "steps", false, "output steps")
	fs.BoolVar(&f.inputProperties, "ip", false, "output input properties")
	fs.BoolVar(&f.outputProperties, "op", false, "output output properties")
}

func (f *buildFieldFlags) FieldMask() *field_mask.FieldMask {
	if f.all {
		ret := &field_mask.FieldMask{}
		for _, p := range proto.GetProperties(reflect.TypeOf(buildbucketpb.Build{})).Prop {
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
			"update_time",
		},
	}

	if f.inputProperties {
		ret.Paths = append(ret.Paths, "input.properties")
	}

	if f.outputProperties {
		ret.Paths = append(ret.Paths, "output.properties")
	}

	if f.steps {
		ret.Paths = append(ret.Paths, "steps")
	}

	if f.tags {
		ret.Paths = append(ret.Paths, "tags")
	}

	return ret
}
