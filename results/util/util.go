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

package util

import (
	resultspb "go.chromium.org/luci/results/proto/v1"
)

// StringPair creates a resultspb.StringPair with the given strings as key/value field values.
func StringPair(k, v string) *resultspb.StringPair {
	return &resultspb.StringPair{Key: k, Value: v}
}
