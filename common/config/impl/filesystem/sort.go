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

package filesystem

import (
	"github.com/luci/luci-go/common/config"
)

type configList []config.Config

func (cl configList) Len() int      { return len(cl) }
func (cl configList) Swap(i, j int) { cl[i], cl[j] = cl[j], cl[i] }
func (cl configList) Less(i, j int) bool {
	if cl[i].ConfigSet < cl[j].ConfigSet {
		return true
	} else if cl[i].ConfigSet > cl[j].ConfigSet {
		return false
	}
	return cl[i].Path < cl[j].Path
}

type projList []config.Project

func (pl projList) Len() int      { return len(pl) }
func (pl projList) Swap(i, j int) { pl[i], pl[j] = pl[j], pl[i] }
func (pl projList) Less(i, j int) bool {
	return pl[i].ID < pl[j].ID
}
