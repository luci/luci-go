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

// Package testdata contains support files for unit tests.
package testdata

// Generate descriptor set with the test proto.
//
// It is used by both Go and Starlark tests.
//
//go:generate protoc -I . -I ../../../../../  -o misc/support/test_descpb.bin misc/support/test.proto
