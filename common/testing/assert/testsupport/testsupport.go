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

// testsupport contains helpers for testing the assertion library.
package testsupport

import (
	"go.chromium.org/luci/common/testing/assert/interfaces"
)

// zeroTB is a do nothing test implementation.
//
// Check and Assert both call methods on the test interface, and this can result in
// really confusing error messages or the test being aborted early. In other to prevent this
// we need to pass in an object that satisfies the test interface but doesn't do anything.
type ZeroTB struct{}

func (ZeroTB) Helper() {}

func (ZeroTB) Log(...any) {}

func (ZeroTB) Fail() {}

func (ZeroTB) FailNow() {}

var _ interfaces.TestingTB = ZeroTB{}
