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

// Package should contains comparisons such as should.Equal.
//
// These are meant to be used with the 'assert' library like:
//
//     import "testing"
//     import . "go.chromium.org/luci/common/testing/assert"
//     import "go.chromium.org/luci/common/testing/assert/should"
//
//     func TestSomething(t *testing.T) {
//         Assert(t, something(), should.Equal("hello"))
//     }
package should
