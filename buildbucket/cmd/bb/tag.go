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
	"fmt"
	"strings"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

type tagsFlag struct {
	tags []*buildbucketpb.StringPair
}

func (f *tagsFlag) Register(fs *flag.FlagSet, help string) {
	flag := stringPairSliceFlag(f.tags)
	fs.Var(&flag, "tags", help)
}

// stringPairSliceFlag is a flag.Value implementation representing a
// []*buildbucketpb.StringPair
//
// TODO(nodir): move this and many other pieces of code from bb command
// to go.chromium.org/buildbucket/cli package.
type stringPairSliceFlag []*buildbucketpb.StringPair

// String returns a comma-separated string representation of the flag values.
func (f *stringPairSliceFlag) String() string {
	ret := &strings.Builder{}
	for i, t := range *f {
		if i > 0 {
			ret.WriteString(", ")
		}
		fmt.Fprintf(ret, "%s:%s", t.Key, t.Value)
	}
	return ret.String()
}

// Set records seeing a flag value.
func (f *stringPairSliceFlag) Set(val string) error {
	parts := strings.SplitN(val, ":", 2)
	if len(parts) == 1 {
		return fmt.Errorf("a tag must have a colon")
	}
	*f = append(*f, &buildbucketpb.StringPair{Key: parts[0], Value: parts[1]})
	return nil
}

// Get retrieves the flag values.
func (f *stringPairSliceFlag) Get() interface{} {
	return ([]*buildbucketpb.StringPair)(*f)
}
