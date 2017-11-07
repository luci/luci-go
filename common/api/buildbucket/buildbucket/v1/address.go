// Copyright 2017 The LUCI Authors.
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

package buildbucket

import (
	"strings"
	"strconv"
	"fmt"
)

// ParseAddress parses a build address returned by Build.Address().
//
// If id is non-zero, project, bucket and builder are zero.
// If bucket, builder and number are non-zero, id is zero.
func ParseAddress(address string) (id int64, project, bucket, builder string, number int, err error) {
	parts := strings.Split(address, "/")
	switch len(parts) {
	case 1:
		id, err = strconv.ParseInt(parts[0], 10, 64)
	case 3:
		var numberStr string
		bucket, builder, numberStr = parts[0], parts[1], parts[2]
		if strings.HasPrefix(bucket, "luci.") {
			project = strings.Split(bucket, ".")[1]
		}
		number, err = strconv.Atoi(numberStr)
	default:
		err = fmt.Errorf("unrecognized build address format %q", address)
	}
	return
}