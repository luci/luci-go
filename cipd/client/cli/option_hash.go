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
	"fmt"
	"strings"

	caspb "go.chromium.org/luci/cipd/api/cipd/v1/caspb"
	"go.chromium.org/luci/cipd/common"
)

////////////////////////////////////////////////////////////////////////////////
// hashOptions mixin.

// allAlgos is used in the flag help text, it is "sha256, sha1, ...".
var allAlgos string

func init() {
	algos := make([]string, 0, len(caspb.HashAlgo_name)-1)
	for i := len(caspb.HashAlgo_name) - 1; i > 0; i-- {
		algos = append(algos, strings.ToLower(caspb.HashAlgo_name[int32(i)]))
	}
	allAlgos = strings.Join(algos, ", ")
}

// hashAlgoFlag adapts caspb.HashAlgo to flag.Value interface.
type hashAlgoFlag caspb.HashAlgo

// String is called by 'flag' package when displaying default value of a flag.
func (ha *hashAlgoFlag) String() string {
	return strings.ToLower(caspb.HashAlgo(*ha).String())
}

// Set is called by 'flag' package when parsing command line options.
func (ha *hashAlgoFlag) Set(value string) error {
	val := caspb.HashAlgo_value[strings.ToUpper(value)]
	if val == 0 {
		return makeCLIError("unknown hash algo %q, should be one of: %s", value, allAlgos)
	}
	*ha = hashAlgoFlag(val)
	return nil
}

// hashOptions defines -hash-algo flag that specifies hash algo to use for
// constructing instance IDs.
//
// Default value is given by common.DefaultHashAlgo.
//
// Not all algos may be accepted by the server.
type hashOptions struct {
	algo hashAlgoFlag
}

func (opts *hashOptions) registerFlags(f *flag.FlagSet) {
	opts.algo = hashAlgoFlag(common.DefaultHashAlgo)
	f.Var(&opts.algo, "hash-algo", fmt.Sprintf("Algorithm to use for deriving package instance ID, one of: %s", allAlgos))
}

func (opts *hashOptions) hashAlgo() caspb.HashAlgo {
	return caspb.HashAlgo(opts.algo)
}
