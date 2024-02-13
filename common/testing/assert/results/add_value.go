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

package results

import "fmt"

func addValue(result *Result, name string, val interface{}) {
	result.values = append(result.values, value{
		name:  name,
		value: val,
	})
}

func addValuef(result *Result, name string, format string, args ...interface{}) {
	if len(args) == 0 {
		result.values = append(result.values, value{name, verbatimString(format)})
	} else {
		result.values = append(result.values, value{name, verbatimString(fmt.Sprintf(format, args...))})
	}
}
