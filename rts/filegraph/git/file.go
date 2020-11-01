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

	"go.chromium.org/luci/common/logging"
)

// GraphFile is a file that stores a Graph.
type GraphFile struct {
	Graph

	// Name is the filename.
	Name string

	// Commit is the last synced commit.
	Commit string
}

// Read reads the graph from the file.
func (f *GraphFile) Read() error {
	// TODO(crbug.com/1136280): implement.
	return nil
}

// Write writes the graph to the file.
func (f *GraphFile) Write() error {
	// TODO(crbug.com/1136280): implement.
	return nil
}

// Sync synchronizes the graph with commits between f.Commit and tillRev,
// and saves the updated graph to the file.
//
// Each 10k commits, flushes the state to disk and logs the number of processed
// commits.
//
// TODO(nodir): remove the limitation that it reads only first commit parents.
func (f *GraphFile) Sync(ctx context.Context, repoDir, tillRev string) error {
	processed := 0
	dirty := false
	err := readLog(ctx, repoDir, f.Commit, tillRev, func(c commit) error {
		if err := f.addCommit(c); err != nil {
			return err
		}
		f.Commit = c.Hash
		dirty = true

		processed++
		if processed%10000 == 0 {
			logging.Infof(ctx, "processed %d commits so far", processed)
			if err := f.Write(); err != nil {
				return err
			}
			dirty = false
		}
		return nil
	})
	if err != nil {
		return err
	}

	if dirty {
		// The last attempt is critical.
		return f.Write()
	}
	return nil
}
