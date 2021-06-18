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

package state

import (
	"go.chromium.org/luci/cv/internal/common"
)

type clidsSet map[common.CLID]struct{}

func (c clidsSet) addI64(id int64)      { c.add(common.CLID(id)) }
func (c clidsSet) add(clid common.CLID) { c[clid] = struct{}{} }
func (c clidsSet) hasI64(id int64) bool { return c.has(common.CLID(id)) }
func (c clidsSet) has(clid common.CLID) bool {
	_, exists := c[clid]
	return exists
}
func (c clidsSet) delI64(id int64) { delete(c, common.CLID(id)) }

// reset resets the amp to contains just given IDs. Used in tests only.
func (c clidsSet) resetI64(ids ...int64) {
	for id := range c {
		delete(c, common.CLID(id))
	}
	for _, id := range ids {
		c.addI64(id)
	}
}
