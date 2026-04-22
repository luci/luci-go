// Copyright 2026 The LUCI Authors.
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

	"go.chromium.org/luci/cipd/common"
)

////////////////////////////////////////////////////////////////////////////////
// tagsOptions mixin.

// tagList holds an array of '-tag' command line options.
type tagList []string

func (tags *tagList) String() string {
	return "key:value"
}

// Set is called by 'flag' package when parsing command line options.
func (tags *tagList) Set(value string) error {
	err := common.ValidateInstanceTag(value)
	if err != nil {
		return cliErrorTag.Apply(err)
	}
	*tags = append(*tags, value)
	return nil
}

// tagsOptions defines command line arguments for commands that accept a set
// of tags.
type tagsOptions struct {
	tags tagList
}

func (opts *tagsOptions) registerFlags(f *flag.FlagSet) {
	f.Var(&opts.tags, "tag", "A `key:value` tag to attach to the package instance (can be used multiple times).")
}
