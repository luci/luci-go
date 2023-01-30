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

// Package exe implements a client for the LUCI Executable ("luciexe") protocol.
//
// The simplest luciexe is:
//
//	import (
//	  "context"
//
//	  "go.chromium.org/luci/luciexe/exe"
//
//	  bbpb "go.chromium.org/luci/buildbucket/proto"
//	)
//
//	func main() {
//	  exe.Run(func(ctx context.Context, input *bbpb.Build, userArgs []string, send exe.BuildSender) error {
//	    ... do whatever you want here ...
//	    return nil // nil error indicates successful build.
//	  })
//	}
//
// See Also: https://go.chromium.org/luci/luciexe
package exe
