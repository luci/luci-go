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

package build

import (
	"context"

	"google.golang.org/protobuf/proto"
)

// Main implements all the 'command-line' behaviors of the luciexe 'exe'
// protocol, including:
//
//   * parsing command line for "--output", "--help", etc.
//   * parsing stdin (as appropriate) for the incoming Build message
//   * creating and configuring a logdog client to send State evolutions.
//   * Configuring a logdog client from the environment.
//   * Writing Build state updates to the logdog "build.proto" stream.
//   * Start'ing the build in this process.
//   * End'ing the build with the returned error from your function
//
// If `inputMsg` is nil, the top-level properties will be ignored.
//
// If `writeFnptr` and `mergeFnptr` are nil, they're ignored. Otherwise
// they work as they would for MakePropertyModifier.
//
// CLI Arguments parsed:
//   * -h / --help : Print help for this binary (including input/output
//     property type info)
//   * --strict : Enable strict property parsing (see OptStrictInputProperties)
//   * --output : luciexe "output" flag; See
//     https://pkg.go.dev/go.chromium.org/luci/luciexe#hdr-Recursive_Invocation
//   * -- : Any extra arguments after a "--" token are passed to your callback
//     as-is.
//
// Example:
//
//    func main() {
//      input := *MyInputProps{}
//      var writeOutputProps func(context.Context, *MyOutputProps)
//      var mergeOutputProps func(context.Context, *MyOutputProps)
//
//      Main(input, &writeOutputProps, &mergeOutputProps, func(ctx context.Context, args []string, st *build.State) error {
//        // actual build code here, build is already Start'd
//        // input was parsed from build.Input.Properties
//        writeOutputProps(ctx, &MyOutputProps{...})
//        return nil // will mark the Build as SUCCESS
//      })
//    }
//
// NOTE: These types are pretty bad; There's significant opportunity to improve
// them with Go2 generics.
func Main(inputMsg proto.Message, writeFnptr, mergeFnptr interface{}, cb func(context.Context, []string, *State) error) {
	panic("implement me")
}
