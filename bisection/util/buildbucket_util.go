// Copyright 2022 The LUCI Authors.
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

// Package util contains utility functions
package util

import (
	bbpb "go.chromium.org/luci/buildbucket/proto"
)

// Check if a step is compile step.
// For the moment, we only check for the step name.
// In the future, we may want to check for step tag (crbug.com/1353978)
func IsCompileStep(step *bbpb.Step) bool {
	return step.GetName() == "compile"
}
