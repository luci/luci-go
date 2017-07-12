// Copyright 2015 The LUCI Authors.
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

package units

import (
	"fmt"
	"time"
)

var units = []string{"b", "Kib", "Mib", "Gib", "Tib", "Pib", "Eib", "Zib", "Yib"}

// Size represents a size in bytes that knows how to print itself.
type Size int64

func (s Size) String() string {
	return SizeToString(int64(s))
}

// SizeToString pretty prints file size (given as number of bytes).
func SizeToString(s int64) string {
	v := float64(s)
	i := 0
	for ; i < len(units); i++ {
		if v < 1024. {
			break
		}
		v /= 1024.
	}
	if i == 0 {
		return fmt.Sprintf("%d%s", s, units[i])
	}
	if v >= 10 {
		return fmt.Sprintf("%.1f%s", v, units[i])
	}
	return fmt.Sprintf("%.2f%s", v, units[i])
}

// Round rounds a time.Duration at round.
func Round(value time.Duration, resolution time.Duration) time.Duration {
	if value < 0 {
		value -= resolution / 2
	} else {
		value += resolution / 2
	}
	return value / resolution * resolution
}
