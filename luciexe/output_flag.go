// Copyright 2020 The LUCI Authors.
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

package luciexe

import (
	"flag"
	"strings"

	bbpb "go.chromium.org/luci/buildbucket/proto"
)

const (
	// OutputCLIArg is the CLI argument to luciexe binaries to instruct them to
	// dump their final Build message. The value of this flag must be an absolute
	// path to a file which doesn't exist in a directory which does (and which the
	// luciexe binary has access to write in). See Output*FileExt for valid
	// extensions.
	OutputCLIArg = "--output"
)

// OutputFlag can be used as a flag.Value to parse the OutputCLIArg as part of
// your FlagSet.
type OutputFlag struct {
	Path  string
	Codec BuildFileCodec
}

var _ flag.Value = (*OutputFlag)(nil)

// Set implements flag.Value.
func (o *OutputFlag) Set(value string) error {
	codec, err := BuildFileCodecForPath(value)
	if err != nil {
		return err
	}

	o.Path = value
	o.Codec = codec
	return nil
}

func (o *OutputFlag) String() string {
	if o == nil {
		return ""
	}
	return o.Path
}

// Write writes the build message to this output file with the appropriate
// encoding.
func (o *OutputFlag) Write(build *bbpb.Build) error {
	return WriteBuildFile(o.Path, build)
}

// AddOutputFlagToSet adds a OutputFlag to the FlagSet with OutputCLIArg and
// a useful helpstring.
//
// Returns the added *OutputFlag. The default value has a Codec of
// BuildFileNone.
func AddOutputFlagToSet(fs *flag.FlagSet) *OutputFlag {
	ret := &OutputFlag{Codec: buildFileCodecNoop{}}
	fs.Var(ret, strings.TrimLeft(OutputCLIArg, "-"),
		"a path to a `build_file` to write the final build.proto into. Valid file extensions: "+validCodecExtsStr)
	return ret
}
