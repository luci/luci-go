// Copyright 2021 The LUCI Authors.
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

// Package updatertest provides test helpers for Gerrit CL Updater.
package updatertest

import (
	"fmt"
	"sort"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/cv/internal/gerrit/updater"
)

// PFilter returns payloads of Gerrit Updater tasks.
func PFilter(in tqtesting.TaskList) (out RefreshGerritCLPayloads) {
	for _, t := range in.SortByETA() {
		if t.Class == updater.TaskClass {
			out = append(out, t.Payload.(*updater.RefreshGerritCL))
		}
	}
	return
}

// ChangeNumbers returns change numbers from payloads of Gerrit Updater tasks.
//
// Panics if payloads are not from the same host.
func ChangeNumbers(in tqtesting.TaskList) []int64 {
	sorted := PFilter(in).SortByChangeNumber()
	out := make([]int64, len(sorted))
	hosts := stringset.New(len(sorted))
	for i, p := range sorted {
		out[i] = p.GetChange()
		hosts.Add(p.GetHost())
	}
	if hosts.Len() > 1 {
		panic(fmt.Errorf("different hosts %s not allowed", hosts))
	}
	return out
}

type RefreshGerritCLPayloads []*updater.RefreshGerritCL

// SortByChangeNumber sorts in place and returns itself.
func (r RefreshGerritCLPayloads) SortByChangeNumber() RefreshGerritCLPayloads {
	sort.Slice(r, func(i, j int) bool { return r[i].GetChange() < r[j].GetChange() })
	return r
}
