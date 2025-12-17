// Copyright 2025 The LUCI Authors.
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

package pkg

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
)

type repoProbe struct {
	// sentinel is the name of the file to look for
	sentinel string
	// locatedInRepoParent indicates that the sentinel file will actually be present in the
	// directory containing the repo rather than the root of the repo
	locatedInRepoParent bool
}

// Used to discover boundaries of repositories.
var repoProbes = []repoProbe{{".git", false}, {".citc", true}}

// findRoot, given a directory path on disk, finds the closest repository or
// volume root directory and returns it as an absolute path.
//
// If given a markerFile, will stop searching if finds a directory that contains
// this file, returning (dir path, true, nil) in that case.
//
// If given a stopDir, will also stop when reaching this directory, returning
// (stopDir, false, nil) if it happens.
func findRoot(dir, markerFile, stopDir string, cache *statCache) (string, bool, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", false, err
	}
	if stopDir != "" {
		if stopDir, err = filepath.Abs(stopDir); err != nil {
			return "", false, err
		}
	}

	if cache == nil {
		cache = unsyncStatCache()
	}

	var probes []repoProbe
	if markerFile == "" {
		probes = repoProbes
	} else {
		// Note the order is important: need to probe for the marker file before
		// probing .git in case the marker file is at the repo root.
		probe := repoProbe{markerFile, false}
		probes = append(make([]repoProbe, 0, 3), probe)
		probes = append(probes, repoProbes...)
	}

	for {
		for _, probe := range probes {
			probeDir := dir
			if probe.locatedInRepoParent {
				probeDir = filepath.Dir(probeDir)
				if probeDir == dir {
					continue // at the volume root, this probe won't match
				}
			}
			switch err := cache.stat(filepath.Join(probeDir, probe.sentinel)); {
			case err == nil:
				return dir, probe.sentinel == markerFile, nil // found the repository root or the marker file
			case errors.Is(err, os.ErrNotExist):
				// Carry on searching
			default:
				// Some file system error (likely no access).
				return "", false, err
			}
		}

		if stopDir != "" && dir == stopDir {
			return stopDir, false, nil // hit the stop directory
		}

		up := filepath.Dir(dir)
		if up == dir {
			return dir, false, nil // hit the volume root
		}

		dir = up
	}
}

// statCache caches calls to os.Stat.
type statCache struct {
	m     *sync.RWMutex    // if nil, do not lock anything
	cache map[string]error // path => os.Stat error
}

func syncStatCache() *statCache {
	return &statCache{
		m:     &sync.RWMutex{},
		cache: map[string]error{},
	}
}

func unsyncStatCache() *statCache {
	return &statCache{
		cache: map[string]error{},
	}
}

func (s *statCache) stat(path string) error {
	if s.m != nil {
		s.m.RLock()
	}
	err, found := s.cache[path]
	if s.m != nil {
		s.m.RUnlock()
	}
	if found {
		return err
	}

	if s.m != nil {
		s.m.Lock()
		defer s.m.Unlock()
		if err, found := s.cache[path]; found {
			return err
		}
	}

	_, err = os.Stat(path)
	s.cache[path] = err
	return err
}
