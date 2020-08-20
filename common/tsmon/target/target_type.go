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
	"fmt"

	"go.chromium.org/luci/common/flag/flagenum"
	"go.chromium.org/luci/common/tsmon/types"
)

var _ flag.Value = (*targetTypeFlag)(nil)

var (
	// NilType is given if a metric was created without the TargetType specified.
	NilType = types.TargetType{}

	DeviceType = (*NetworkDevice)(nil).Type()
	TaskType   = (*Task)(nil).Type()
)

var targetTypeEnum = flagenum.Enum{
	DeviceType.Name: DeviceType,
	TaskType.Name:   TaskType,
}

type targetTypeFlag struct {
	flags *Flags
}

func (ttf *targetTypeFlag) String() string {
	if ttf.flags != nil {
		return ttf.flags.TargetType.Name
	}
	return DeviceType.Name
}

func (ttf *targetTypeFlag) Set(v string) error {
	targetType, ok := targetTypeEnum[v]
	if !ok {
		return fmt.Errorf("unknown --ts-mon-target-type '%s'", v)
	}
	ttf.flags.TargetType = targetType.(types.TargetType)
	return nil
}
