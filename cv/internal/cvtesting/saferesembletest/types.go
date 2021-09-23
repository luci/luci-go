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

type WithInnerStructNoProto struct {
	X int
	y string
	I NoProtos
}

func NewWithInnerStructNoProto(X int, y string, I NoProtos) WithInnerStructNoProto {
	return WithInnerStructNoProto{X: X, y: y, I: I}
}

type WithInnerStructHasProto struct {
	X int
	y string
	I WithPrivateProto
}

func NewWithInnerStructHasProto(X int, y string, I WithPrivateProto) WithInnerStructHasProto {
	return WithInnerStructHasProto{X: X, y: y, I: I}
}

type WithPrivateInnerStructNoProto struct {
	X int
	y string
	i NoProtos
}

func NewWithPrivateInnerStructNoProto(X int, y string, i NoProtos) WithPrivateInnerStructNoProto {
	return WithPrivateInnerStructNoProto{X: X, y: y, i: i}
}

type WithPrivateStructHasProto struct {
	X int
	y string
	i WithPrivateProto
}

func NewWithPrivateStructHasProto(X int, y string, i WithPrivateProto) WithPrivateStructHasProto {
	return WithPrivateStructHasProto{X: X, y: y, i: i}
}

type WithInnerStructSliceNoProto struct {
	X  int
	y  string
	Is []NoProtos
}

func NewWithInnerStructSliceNoProto(X int, y string, Is []NoProtos) WithInnerStructSliceNoProto {
	return WithInnerStructSliceNoProto{X: X, y: y, Is: Is}
}

type WithInnerStructSliceHasProto struct {
	X  int
	y  string
	Is []WithPrivateProto
}

func NewWithInnerStructSliceHasProto(X int, y string, Is []WithPrivateProto) WithInnerStructSliceHasProto {
	return WithInnerStructSliceHasProto{X: X, y: y, Is: Is}
}

type WithPrivateInnerStructSliceNoProto struct {
	X  int
	y  string
	is []NoProtos
}

func NewWithPrivateInnerStructSliceNoProto(X int, y string, is []NoProtos) WithPrivateInnerStructSliceNoProto {
	return WithPrivateInnerStructSliceNoProto{X: X, y: y, is: is}
}

type WithPrivateInnerStructSliceHasProto struct {
	X  int
	y  string
	is []WithPrivateProto
}

func NewWithPrivateInnerStructSliceHasProto(X int, y string, is []WithPrivateProto) WithPrivateInnerStructSliceHasProto {
	return WithPrivateInnerStructSliceHasProto{X: X, y: y, is: is}
}
