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

import (
	"context"
)

type commit struct {
	Hash         string
	ParentHashes []string
	Files        []fileChange
}

type fileChange struct {
	// Status is the file change type: 'A', 'C', 'D', 'R', 'M', etc.
	// For more info, see --diff-filter in https://git-scm.com/docs/git-diff
	Status byte
	Path   string
	Path2  string // populated if Status is 'R'
}

// readLog calls the callback for each commit reachable from `rev` and not
// reachable from `exclude`. The order of commits is "reversed", i.e. ancestors
// first.
func readLog(ctx context.Context, repoDir, exclude, rev string, callback func(commit) error) error {
	// TODO(crbug.com/1136280): implement
	panic("not implemented")
}
