// Copyright 2026 The LUCI Authors.
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

package internal

import (
	"cmp"
	"maps"
	"slices"

	"go.chromium.org/luci/cipd/client/cipd/internal/messages"
)

type tagKey struct {
	pkg string
	tag string
}

func mkTagKey(e *messages.VersionCache_Entry) tagKey {
	return tagKey{e.Package, e.Tag}
}

type tagMap map[tagKey]*messages.VersionCache_Entry

func (t tagMap) has(e *messages.VersionCache_Entry) bool {
	return t[mkTagKey(e)] != nil
}

func (t tagMap) sorted(yield func(tagKey, *messages.VersionCache_Entry) bool) {
	keys := slices.AppendSeq(make([]tagKey, 0, len(t)), maps.Keys(t))
	slices.SortFunc(keys, func(a, b tagKey) int {
		return cmp.Or(
			cmp.Compare(a.pkg, b.pkg),
			cmp.Compare(a.tag, b.tag),
		)
	})
	for _, k := range keys {
		if !yield(k, t[k]) {
			return
		}
	}
}

type fileKey struct {
	pkg      string
	instance string
	file     string
}

func mkFileKey(e *messages.VersionCache_FileEntry) fileKey {
	return fileKey{e.Package, e.InstanceId, e.FileName}
}

type fileMap map[fileKey]*messages.VersionCache_FileEntry

func (t fileMap) has(e *messages.VersionCache_FileEntry) bool {
	return t[mkFileKey(e)] != nil
}

func (f fileMap) sorted(yield func(fileKey, *messages.VersionCache_FileEntry) bool) {
	keys := slices.AppendSeq(make([]fileKey, 0, len(f)), maps.Keys(f))
	slices.SortFunc(keys, func(a, b fileKey) int {
		return cmp.Or(
			cmp.Compare(a.pkg, b.pkg),
			cmp.Compare(a.instance, b.instance),
			cmp.Compare(a.file, b.file),
		)
	})
	for _, k := range keys {
		if !yield(k, f[k]) {
			return
		}
	}
}

type refKey struct {
	pkg string
	ref string
}

func mkRefKey(e *messages.VersionCache_RefEntry) refKey {
	return refKey{e.Package, e.Ref}
}

type refMap map[refKey]*messages.VersionCache_RefEntry

func (t refMap) has(e *messages.VersionCache_RefEntry) bool {
	return t[mkRefKey(e)] != nil
}

func (f refMap) sorted(yield func(refKey, *messages.VersionCache_RefEntry) bool) {
	keys := slices.AppendSeq(make([]refKey, 0, len(f)), maps.Keys(f))
	slices.SortFunc(keys, func(a, b refKey) int {
		return cmp.Or(
			cmp.Compare(a.pkg, b.pkg),
			cmp.Compare(a.ref, b.ref),
		)
	})
	for _, k := range keys {
		if !yield(k, f[k]) {
			return
		}
	}
}
