// Copyright 2023 The LUCI Authors.
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

// Command dump_build is a simple CLI debugging toolV which reads a binary Build
// message (with optional zlib compression) and then dumps the decoded build as
// JSONPB to stdout.
package main

import (
	"bytes"
	"compress/zlib"
	"io"
	"os"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	bbpb "go.chromium.org/luci/buildbucket/proto"
)

func main() {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		panic(err)
	}

	if raw[0] == 0x78 { // zlib magic
		r, err := zlib.NewReader(bytes.NewReader(raw))
		if err != nil {
			panic(err)
		}
		raw, err = io.ReadAll(r)
		if err != nil {
			panic(err)
		}
	}

	build := &bbpb.Build{}
	if err = proto.Unmarshal(raw, build); err != nil {
		panic(err)
	}

	if _, err = os.Stdout.WriteString(protojson.Format(build)); err != nil {
		panic(err)
	}
	if _, err = os.Stdout.WriteString("\n"); err != nil {
		panic(err)
	}
}
