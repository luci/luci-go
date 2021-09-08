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

// Package saferesembletest exists to test better cvtesting.SafeShouldResemble
package saferesembletest

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

type NoProtos struct {
	a int
	A time.Time
}

func NewNoProtos(a int, A time.Time) NoProtos {
	return NoProtos{a: a, A: A}
}

type WithPrivateProto struct {
	A int
	b string
	t *timestamppb.Timestamp
}

func NewWithPrivateProto(A int, b string, t *timestamppb.Timestamp) WithPrivateProto {
	return WithPrivateProto{A: A, b: b, t: t}
}
