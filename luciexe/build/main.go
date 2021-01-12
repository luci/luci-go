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

	"github.com/golang/protobuf/proto"
)

// Main implements all the 'command-line' behaviors of the luciexe 'exe'
// protocol, including:
//
//   * parsing command line for "--output", "--help", etc.
//   * parsing stdin (as appropriate) for the incoming Build message
//   * creating and configuring a logdog client to send State evolutions.
//   * Start'ing the build in this process.
//   * End'ing the build with the returned error from your function
//
// cbFn should be a function (using Go2 generic syntax) of the type:
//
//    type Callback[T proto.Message] func(context.Context, *State, T) error
//
// Example:
//
//    func main() {
//      input := *MyInputProps{}
//      Main(input, func(ctx context.Context, st *build.State) error {
//        // actual build code here, build is already Start'd
//        // input was populated from the build.Input.Properties
//        return nil // will mark the Build as SUCCESS
//      })
//    }
func Main(inputMsg proto.Message, cb func(context.Context, *State) error) {
	panic("implement me")
}

// MainPropWriter is like Main but also takes a `modFnptr` which is the pointer
// to a property modifier function as described by MakePropertyModifier.
//
// Example:
//
//    func main() {
//      input := *MyInputProps{}
//      var output func(context.Context, ProtoWriteAlgorithm, *MyOutputProps)
//      Main(input, &output, func(ctx context.Context, st *build.State) error {
//        // actual build code here, build is already Start'd
//        // input was populated from the build.Input.Properties
//        return nil // will mark the Build as SUCCESS
//      })
//    }
func MainWithOutput(inputMsg proto.Message, modFnptr interface{}, cb func(context.Context, *State)) {
	panic("implement me")
}
