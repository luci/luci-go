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

package gitsource

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"

	"go.chromium.org/luci/common/data/stringset"
)

type treeEntry struct {
	mode int64
	kind ObjectKind
	hash string
}
type tree map[string]treeEntry

func (b *batchProc) catFileTree(ctx context.Context, commit, path string) (tree, error) {
	kind, data, err := b.catFile(ctx, fmt.Sprintf("%s:%s", commit, path))
	if err != nil {
		return nil, err
	}
	if kind != TreeKind {
		return nil, fmt.Errorf("not a tree: %s", kind)
	}

	ret := make(tree)

	// parse the tree
	reader := bufio.NewReader(bytes.NewReader(data))
	for {
		modeName, err := reader.ReadBytes(0)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("could not read until null: %w", err)
		}
		hash := make([]byte, sha1.Size)
		if _, err := io.ReadFull(reader, hash); err != nil {
			return nil, fmt.Errorf("could not read hash: %w", err)
		}

		modeIdx := bytes.IndexByte(modeName, ' ')
		if modeIdx < 0 {
			return nil, fmt.Errorf("could not parse modeName: %q", modeName)
		}

		modeStr := string(modeName[:modeIdx])
		// Skip leading space and trailing NULL
		nameStr := string(modeName[modeIdx+1 : len(modeName)-1])

		mode, err := strconv.ParseInt(modeStr, 8, 64)
		if err != nil {
			return nil, fmt.Errorf("could not parse mode: %q: %w", modeStr, err)
		}

		kind := UnknownKind
		if (mode & 040000) != 0 {
			kind = TreeKind
		} else if (mode & 012000) != 0 {
			kind = SymlinkKind
		} else if (mode & 0100777) == mode {
			kind = BlobKind
		}

		ret[nameStr] = treeEntry{mode, kind, hex.EncodeToString(hash)}
	}

	return ret, nil
}

func (b *batchProc) catFileTreeRecursive(ctx context.Context, commit, repoRelPath string) (tree, error) {
	ret := make(tree)

	treeHashes := stringset.New(10)
	toProcess := []string{repoRelPath}
	for len(toProcess) > 0 {
		item := toProcess[0]
		toProcess = toProcess[1:]

		tree, err := b.catFileTree(ctx, commit, item)
		if err != nil {
			return nil, err
		}

		for _, name := range slices.Sorted(maps.Keys(tree)) {
			key := path.Join(item, name)
			if strings.Contains(name, "/") {
				return nil, fmt.Errorf("git tree %s:%s - entry %q contains /", commit, item, name)
			}

			entry := tree[name]
			ret[key] = entry
			if entry.kind == TreeKind {
				if treeHashes.Has(entry.hash) {
					return nil, fmt.Errorf("git tree %s:%s - entry %q is recursive on %q", commit, item, name, entry.hash)
				}
				treeHashes.Add(entry.hash)
				toProcess = append(toProcess, key)
			}
		}
	}

	return ret, nil
}
