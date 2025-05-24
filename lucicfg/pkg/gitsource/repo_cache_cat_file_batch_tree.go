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
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"syscall"

	"go.chromium.org/luci/common/data/stringset"
)

type treeEntry struct {
	name string
	mode int64
	kind ObjectKind
	hash string
}
type tree []treeEntry

func (b *batchProc) catFileTree(ctx context.Context, commit, path string) (tree, error) {
	kind, data, err := b.catFile(ctx, fmt.Sprintf("%s:%s", commit, path))
	if err != nil {
		return nil, err
	}
	if kind != TreeKind {
		return nil, fmt.Errorf("not a tree: %s", kind)
	}

	var ret tree

	// parse the tree
	reader := bufio.NewReader(bytes.NewReader(data))
	for {
		modeName, err := reader.ReadBytes(0)
		if errors.Is(err, io.EOF) {
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
		switch mode {
		case syscall.S_IFDIR:
			kind = TreeKind
		case syscall.S_IFLNK:
			kind = SymlinkKind
		case syscall.S_IFDIR + syscall.S_IFLNK:
			kind = GitLinkKind
		case syscall.S_IFREG + 0o644, syscall.S_IFREG + 0o755:
			// don't need to distinguish +x, original mode is also returned.
			kind = BlobKind
		}

		ret = append(ret, treeEntry{nameStr, mode, kind, hex.EncodeToString(hash)})
	}

	return ret, nil
}

func (b *batchProc) catFileTreeWalk(ctx context.Context, commit, repoRelPath string, loopCheck stringset.Set, cb func(repoRelPath string, dirContent tree) (skipIdxes map[int]struct{}, err error)) error {
	tree, err := b.catFileTree(ctx, commit, repoRelPath)
	if err != nil {
		return fmt.Errorf("catFileTreeWalk: %s:%s: catFileTree returned error: %w", commit, repoRelPath, err)
	}

	skipIdxs, err := cb(repoRelPath, tree)
	if err != nil {
		return fmt.Errorf("catFileTreeWalk: %s:%s: callback returned error: %w", commit, repoRelPath, err)
	}

	var subLoopCheck stringset.Set
	for i, entry := range tree {
		if _, has := skipIdxs[i]; has {
			continue
		}
		if entry.kind == TreeKind {
			if strings.Contains(entry.name, "/") {
				return fmt.Errorf("catFileTreeWalk: %s:%s: entry %q contains /", commit, repoRelPath, entry.name)
			}
			if loopCheck.Has(entry.hash) {
				return fmt.Errorf("catFileTreeWalk: %s:%s: entry %q is recursive on %q", commit, repoRelPath, entry.name, entry.hash)
			}
			if subLoopCheck == nil {
				subLoopCheck = stringset.New(1)
			} else {
				subLoopCheck = loopCheck.Dup()
			}
			subLoopCheck.Add(entry.hash)
			if err := b.catFileTreeWalk(ctx, commit, path.Join(repoRelPath, entry.name), subLoopCheck, cb); err != nil {
				return err
			}
			subLoopCheck.Del(entry.hash)
		}
	}

	return nil
}
