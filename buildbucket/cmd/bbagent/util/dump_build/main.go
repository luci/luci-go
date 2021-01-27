// Copyright 2021 The LUCI Authors.
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
	"compress/zlib"
	"io/ioutil"
	"os"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func main() {
	decompressing, err := zlib.NewReader(os.Stdin)
	if err != nil {
		panic(err)
	}
	decompressed, err := ioutil.ReadAll(decompressing)
	if err != nil {
		panic(err)
	}

	b := &bbpb.Build{}
	if err := proto.Unmarshal(decompressed, b); err != nil {
		panic(err)
	}
	cb := &bbpb.Build{
		Status: b.Status,
		Steps:  b.Steps,
	}

	m := &protojson.MarshalOptions{Indent: "  "}
	out, err := m.Marshal(cb)
	if err != nil {
		panic(err)
	}

	os.Stdout.Write(out)
}
