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

package git

import "context"

// Sync synchronizes the graph with commits in g.Commit..tillRev.
//
// If the callback is not nil, it is called after each commit is processed.
func (g *Graph) Sync(ctx context.Context, repoDir, tillRev string, callback func() error) error {
	// TODO(crbug.com/1136280): implement.
	return nil
}
