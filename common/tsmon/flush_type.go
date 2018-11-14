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

package tsmon

import (
	"flag"

	"go.chromium.org/luci/common/flag/flagenum"
)

const (
	// FlushManual requires the user to flush manually.
	FlushManual = FlushType("manual")
	// FlushAuto automatically flushes at a periodic flush interval.
	FlushAuto = FlushType("auto")
)

// FlushType is a flush type enumeration.
type FlushType string

func (ft *FlushType) String() string {
	return flushTypeEnum.FlagString(ft)
}

// Set implements flag.Value.
func (ft *FlushType) Set(v string) error {
	return flushTypeEnum.FlagSet(ft, v)
}

var _ flag.Value = (*FlushType)(nil)

var flushTypeEnum = flagenum.Enum{
	"manual": FlushManual,
	"auto":   FlushAuto,
}
