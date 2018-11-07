// Copyright 2015 The LUCI Authors.
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

package milo

import (
	"bytes"
	"encoding/json"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/ptypes/struct"
)

// ContentTypeAnnotations is a stream content type for annotation streams.
const ContentTypeAnnotations = "text/x-chrome-infra-annotations; version=2"

// ExtractProperties returns a flat list of properties from a tree of steps.
// If multiple step nodes have a property of the same name, the last one in the
// preorder traversal wins.
func ExtractProperties(s *Step) (*structpb.Struct, error) {
	finals := map[string]json.RawMessage{}

	var extract func(s *Step)
	extract = func(s *Step) {
		for _, p := range s.Property {
			finals[p.Name] = json.RawMessage(p.Value)
		}
		for _, substep := range s.GetSubstep() {
			if ss := substep.GetStep(); ss != nil {
				extract(ss)
			}
		}
	}
	extract(s)

	buf, err := json.Marshal(finals)
	if err != nil {
		panic("failed to marshal in-memory map to JSON")
	}

	ret := &structpb.Struct{}
	if err := jsonpb.Unmarshal(bytes.NewReader(buf), ret); err != nil {
		return nil, err
	}
	return ret, nil
}
