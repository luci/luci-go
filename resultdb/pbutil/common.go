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

package pbutil

import (
	"reflect"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// ParseResourceName extracts individual components of a slash-separated
// resource name.
// format must be a slash-separated path with even number of components,
// where each 2*i-th component is the expected component in name at the same
// position.
// dest is populated with values retrieved from name.
// Example:
//
//  err := ParseResourceName(invocationName, "invocations/{invocation_id}", &invocationID)
func ParseResourceName(name string, format string, dest ...interface{}) error {
	expectedComponents := strings.Split(format, "/")
	if len(dest) != len(expectedComponents)/2 {
		panic("invalid len(dest)")
	}
	actualComponents := strings.SplitN(name, "/", len(expectedComponents))
	if len(actualComponents) < len(expectedComponents) {
		return errors.Reason("does not match format %q", format).Err()
	}
	for i, exp := range expectedComponents {
		actual := actualComponents[i]
		if i%2 == 0 {
			if actual != exp {
				return errors.Reason("does not match format %q", format).Err()
			}
		} else {
			reflect.ValueOf(dest[i/2]).Elem().SetString(actual)
		}
	}
	return nil
}
