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

package cas

import (
	"os"
	"path/filepath"
	"strings"

	"go.chromium.org/luci/common/errors"
)

var osPathSeparator = string(os.PathSeparator)

// InputMerger mergs a series of (root directory, relative paths) produced by
// isolate.ProcessIsolateForCAS(). This can be used to archive either a single
// isolate file or a batch of them.
type InputMerger struct {
	rootDir  string
	relPaths []string
	err      error
}

func (im *InputMerger) Merge(rootDir string, relPaths []string) error {
	newRoot, err := findAncestor(im.rootDir, rootDir)
	if err != nil {
		im.setErrorIfNil(err)
		return err
	}
	lhsRelPaths, err := rebaseToNewRoot(im.rootDir, im.relPaths, newRoot)
	if err != nil {
		im.setErrorIfNil(err)
		return err
	}
	rhsRelPaths, err := rebaseToNewRoot(rootDir, relPaths, newRoot)
	if err != nil {
		im.setErrorIfNil(err)
		return err
	}
	im.rootDir = newRoot
	// Do not free the memory since we will reuse it soon.
	im.relPaths = im.relPaths[:0]
	seen := make(map[string]bool)

	doMerge := func(relPaths []string) {
		for _, rp := range relPaths {
			if _, ok := seen[rp]; ok {
				continue
			}
			seen[rp] = true
			im.relPaths = append(im.relPaths, rp)
		}
	}
	doMerge(lhsRelPaths)
	doMerge(rhsRelPaths)
	return nil
}

func (im *InputMerger) Finalize() (string, []string, error) {
	if im.err != nil {
		return "", nil, im.err
	}
	return im.rootDir, im.relPaths, nil
}

func (im *InputMerger) setErrorIfNil(e error) {
	if im.err == nil {
		im.err = e
	}
}

func findAncestor(lhs, rhs string) (string, error) {
	anc := lhs
	for {
		rel, err := filepath.Rel(lhs, rhs)
		if err != nil {
			return "", nil
		}
		if !strings.HasPrefix(rel, "..") {
			break
		}
		newAnc := filepath.Dir(anc)
		if newAnc == anc {
			return "", errors.Reason("failed to find the ancestor for lhs=%s rhs=%s", lhs, rhs).Err()
		}
		anc = newAnc
	}
	return anc, nil
}

func rebaseToNewRoot(curRootDir string, curRelPaths []string, newRootDir string) ([]string, error) {
	newRelPaths := make([]string, len(curRelPaths))
	for i, rp := range curRelPaths {
		isDir := strings.HasSuffix(rp, osPathSeparator)
		absPath := filepath.Join(curRootDir, rp)
		if !filepath.IsAbs(absPath) {
			return nil, errors.Reason("not an absolute path: %s", absPath).Err()
		}
		newRel, err := filepath.Rel(newRootDir, absPath)
		if err != nil {
			return nil, err
		}
		if isDir && !strings.HasSuffix(newRel, osPathSeparator) {
			newRel += osPathSeparator
		}
		newRelPaths[i] = newRel
	}
	return newRelPaths, nil
}
