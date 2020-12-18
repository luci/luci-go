// Copyright 2020 The LUCI Authors.
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

package run

import (
	"fmt"

	"go.chromium.org/luci/gae/service/datastore"
)

// NewQueryWithLUCIProject returns a query for Run entities that belongs to
// given LUCI Project.
func NewQueryWithLUCIProject(project string) *datastore.Query {
	return datastore.NewQuery(RunKind).
		// ID starts with the LUCI Project name followed by '/' and 13 digit number.
		// So it must be lexicographically greater than "project/0" and less than
		// "project/:".
		Gt("__key__", fmt.Sprintf("%s/0", project)).
		Lt("__key__", fmt.Sprintf("%s/:", project))
}
