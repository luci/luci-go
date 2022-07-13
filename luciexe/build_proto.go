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

package luciexe

import (
	"go.chromium.org/luci/buildbucket/protoutil"
)

const (
	// BuildProtoLogName is the Build.Step.Log.Name for sub-lucictx programs.
	BuildProtoLogName = "$build.proto"

	// BuildProtoContentType is the ContentType of the build.proto LogDog datagram
	// stream.
	BuildProtoContentType = protoutil.BuildMediaType

	// BuildProtoZlibContentType is the ContentType of the compressed
	// build.proto LogDog datagram stream. It's the same as BuildProtoContentType
	// except it's compressed with zlib.
	BuildProtoZlibContentType = BuildProtoContentType + "; encoding=zlib"

	// BuildProtoStreamSuffix is the logdog stream name suffix for sub-lucictx
	// programs to output their build.proto stream to.
	//
	// TODO(iannucci): Maybe change protocol so that build.proto stream can be
	// opened by the invoking process instead of the invoked process.
	BuildProtoStreamSuffix = "build.proto"
)
