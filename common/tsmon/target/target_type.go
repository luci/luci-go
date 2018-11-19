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

package target

import (
	"flag"

	"go.chromium.org/luci/common/flag/flagenum"
)

// Type is a target type enumeration.
type Type string

var _ flag.Value = (*Type)(nil)

const (
	// DeviceType is a device target type.
	DeviceType = Type("device")
	// TaskType represents a task target type.
	TaskType = Type("task")
)

var targetTypeEnum = flagenum.Enum{
	"device": DeviceType,
	"task":   TaskType,
}

func (tt *Type) String() string {
	return targetTypeEnum.FlagString(tt)
}

// Set implements flag.Value.
func (tt *Type) Set(v string) error {
	return targetTypeEnum.FlagSet(tt, v)
}
