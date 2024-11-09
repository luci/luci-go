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

package cli

import (
	"flag"

	"go.chromium.org/luci/common/flag/flagenum"

	"go.chromium.org/luci/logdog/client/coordinator"
)

type trinaryValue coordinator.QueryTrinary

var _ flag.Value = (*trinaryValue)(nil)

var trinaryFlagEnum = flagenum.Enum{
	"":    coordinator.Both,
	"yes": coordinator.Yes,
	"no":  coordinator.No,
}

func (v trinaryValue) Trinary() coordinator.QueryTrinary {
	return coordinator.QueryTrinary(v)
}

func (v *trinaryValue) String() string {
	return trinaryFlagEnum.FlagString(*v)
}

func (v *trinaryValue) Set(s string) error {
	var tv coordinator.QueryTrinary
	if err := trinaryFlagEnum.FlagSet(&tv, s); err != nil {
		return err
	}
	*v = trinaryValue(tv)
	return nil
}
