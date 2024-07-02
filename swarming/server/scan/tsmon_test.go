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

package scan

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/common/tsmon/types"
)

// TODO: Move this into monitor.Fake.

func GlobalValues(t *testing.T, cells [][]types.Cell, metric string) []string {
	var out []string
	for _, cellSlice := range cells {
		for _, cell := range cellSlice {
			if cell.Name != metric {
				continue
			}
			tg, _ := cell.Target.(*target.Task)
			if tg == nil || tg.HostName != "global" {
				t.Errorf("Expecting global target, got %v", cell.Target)
				continue
			}
			var parts []string
			for _, fv := range cell.FieldVals {
				if val, ok := fv.(string); ok {
					parts = append(parts, val)
				} else {
					parts = append(parts, "???")
				}
			}
			parts = append(parts, fmt.Sprintf("%v", cell.Value))
			out = append(out, strings.Join(parts, ":"))
		}
	}
	sort.Strings(out)
	return out
}

func PerBotValues(t *testing.T, cells [][]types.Cell, metric string) []string {
	var out []string
	for _, cellSlice := range cells {
		for _, cell := range cellSlice {
			if cell.Name != metric {
				continue
			}
			tg, _ := cell.Target.(*target.Task)
			if tg == nil || !strings.HasPrefix(tg.HostName, "autogen:") {
				t.Errorf("Expecting bot target, got %v", cell.Target)
				continue
			}
			if len(cell.FieldVals) != 0 {
				t.Errorf("Unexpected field vals %v", cell.FieldVals)
				continue
			}
			out = append(out, fmt.Sprintf("%s:%v", strings.TrimPrefix(tg.HostName, "autogen:"), cell.Value))
		}
	}
	sort.Strings(out)
	return out
}
