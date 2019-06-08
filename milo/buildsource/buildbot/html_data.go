// Copyright 2016 The LUCI Authors.
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

package buildbot

// TestCases is put here because _test.go files are sometimes not built.
//
// TODO(maruel): What?
var TestCases = []struct {
	Builder string
	Build   int
}{
	{"CrWinGoma", 30608},
	{"chromium_presubmit", 426944},
	{"newline", 1234},
	{"gerritCL", 1234},
	{"win_chromium_rel_ng", 246309},
}
