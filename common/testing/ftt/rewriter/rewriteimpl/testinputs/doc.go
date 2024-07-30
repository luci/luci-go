// Copyright 2024 The LUCI Authors.
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

// Package testinputs is a valid Go package which contains test files (and only
// test files) using a variety of testing functionality.
//
// Each file contains a single, valid, Test function, and serves as input to the
// tests in rewriterimpl, which will compare them to the same-named file in the
// test_outputs package, which is a sibling of this one.
package testinputs
