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

package main

import (
	"fmt"

	"github.com/golang/protobuf/jsonpb"
	structpb "github.com/golang/protobuf/ptypes/struct"

	pb "go.chromium.org/luci/buildbucket/proto"
)

func main() {
	build := &pb.Build{
		Input: &pb.Build_Input{
			Properties: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"someKey": &structpb.Value{
						Kind: &structpb.Value_NumberValue{
							NumberValue: 5.0,
						},
					},
				},
			},
		},
	}
	fmt.Println((&jsonpb.Marshaler{Indent: "  "}).MarshalToString(build))
}
