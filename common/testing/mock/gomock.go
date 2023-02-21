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

// Package mock has utility functions for gomock.
package mock

import (
	"fmt"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto"
)

type protoEqMatcher struct {
	textDef string
}

func (m *protoEqMatcher) Matches(x any) bool {
	return proto.CompactTextString(x.(proto.Message)) == m.textDef
}

func (m *protoEqMatcher) String() string {
	return fmt.Sprintf("is equivalent to proto %q", m.textDef)
}

// EqProto is an equivalent of gomock.Eq for protobuf messages.
func EqProto(m proto.Message) gomock.Matcher {
	return &protoEqMatcher{textDef: proto.CompactTextString(m)}
}
