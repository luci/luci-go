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

// Package structuraldiff provides best-effort prettyprinting and diffing of
// pretty-printed values.
//
// The output of any method in this package is not remotely stable. Use at your
// own risk.
//
// As a quick word of caution, both DebugDump and DebugEqual are intentionally
// abstraction-breaking and will NOT ignore unexported fields. Other packages
// like go-cmp will ignore unexported fields and will make you pass in an option
// to prove that you own the struct in question and can look at its fields.
//
// We don't do that. You can always see everything.
package structuraldiff
