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

// Package interfaces contains all the interfaces necessary for the different
// components of the testing library like TestingTB.
//
// If you are just using the assert or should libraries, you shouldn't
// need to worry about this package.
package interfaces

// TestingTB exposes a subset of the testing.T interface from the standard
// library.
//
// Keep this in sync with results.FakeTB.
type TestingTB interface {
	Helper()
	Log(...any)
	Fail()
	FailNow()
}