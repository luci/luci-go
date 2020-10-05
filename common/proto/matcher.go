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

package proto

import (
	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto"
)

type matcherEq struct {
	expected proto.Message
}

var _ gomock.Matcher = (*matcherEq)(nil)

// MatcherEqual returns a matcher that matches on protobuf equality.
// Note: reflect.DeepEqual can't be used with protobuf messages as it may yield
// unexpected results.
func MatcherEqual(m proto.Message) gomock.Matcher {
	return &matcherEq{
		expected: m,
	}
}

// Matches returns true if x is proto message and if it matches expected
// message.
func (m *matcherEq) Matches(x interface{}) bool {
	message, ok := x.(proto.Message)
	if !ok {
		return false
	}
	return proto.Equal(m.expected, message)
}

// String returns string representation of expected message.
func (m *matcherEq) String() string {
	return m.expected.String()
}
