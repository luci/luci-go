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

package cfgcache

import (
	"fmt"
)

// File represents a single file or regex file pattern in a pipeline's Files
// list.
//
// Implemented by [*SingleFile] and [*RegexFile].
type File interface {
	// pattern returns the raw pattern (i.e. after `exact:` or `regex:`).
	pattern() string

	// isRegex returns true if [pattern] is a regex.
	isRegex() bool

	// unmarshalFunc is a type erased version of Unmarshal.
	unmarshalFunc() (genericUnmarshalFunc, error)

	// recordInSnapshot records an unmarshaled value for resolved `path` in snap.
	//
	// Called with `nil` once per SingleFile to allow SingleFiles to record the
	// resolved `path`, even when the file is missing. This isn't done for
	// RegexFiles because presence is recorded by presence in the map.
	recordInSnapshot(snap *Snapshot, path string, val any)
}

// SingleFile describes a single configuration file within a config set.
type SingleFile[T any] struct {
	// Path is the file path within the config set (e.g. "cool.cfg" or
	// "${appid}.cfg").
	//
	// Required.
	Path string

	// NewDefault is used to mint a new default value returned by
	// [SingleFile.Get] when this file is not present in the [Snapshot].
	//
	// If nil and this file is not present in the [Snapshot], [SingleFile.Get]
	// returns the zero value of T.
	//
	// This is a function rather than a static `T` to ensure that [Transform]
	// always gets a fresh copy (to allow [Transform] to mutate this as if it
	// was a newly-unmarshaled value from LUCI Config).
	NewDefault func() T

	// Unmarshal, if set, will be called on the raw data returned from LUCI
	// Config.
	//
	// If unset:
	//   - If T is a concrete proto.Message (e.g. *MyMessage), the config will be
	//     decoded as text-encoded proto with default options (namely, rejecting
	//     unknown fields).
	//   - If T is string or []byte, the raw content is passed through.
	//   - For any other type T, Unmarshal is required.
	Unmarshal UnmarshalFunc[T]
}

func (f *SingleFile[T]) isRegex() bool { return false }

func (f *SingleFile[T]) pattern() string { return f.Path }

func (f *SingleFile[T]) recordInSnapshot(snap *Snapshot, path string, val any) {
	snap.singleFiles[f] = singleFileEntry{
		resolvedPath: path,
		val:          val,
	}
}

func (f *SingleFile[T]) unmarshalFunc() (genericUnmarshalFunc, error) {
	return f.Unmarshal.asGeneric()
}

// Get retrieves the unmarshaled value of f from snap.
//
// If f was not present in the config set, Get returns f.NewDefault() (or the
// zero value of T if f.NewDefault is nil).
//
// Panics if f was not registered in the pipeline that produced snap.
func (f *SingleFile[T]) Get(snap *Snapshot) T {
	entry, ok := snap.singleFiles[f]
	if !ok {
		panic(fmt.Sprintf("SingleFile %q is not registered in this pipeline", f.Path))
	}
	if entry.val != nil {
		return entry.val.(T)
	}
	if f.NewDefault != nil {
		return f.NewDefault()
	}
	var zero T
	return zero
}

// ResolvedPath returns the concrete file path for f in snap (with any template
// variables such as "${appid}" expanded).
//
// This can be passed to [validation.Context.SetFile] when attributing
// validation errors or warnings to f in a multi-file pipeline.
//
// Panics if f was not registered in the pipeline that produced snap.
func (f *SingleFile[T]) ResolvedPath(snap *Snapshot) string {
	entry, ok := snap.singleFiles[f]
	if !ok {
		panic(fmt.Sprintf("SingleFile %q is not registered in this pipeline", f.Path))
	}
	return entry.resolvedPath
}

// RegexFile describes a set of configuration files within a config set whose
// paths match a regular expression.
type RegexFile[T any] struct {
	// PathRegex is a regular expression matching file paths within the config
	// set (e.g. `shards/.*\.cfg` or `${appid}/.*\.cfg`).
	//
	// Anchors (^ and $) are added automatically if not present.
	//
	// Required.
	PathRegex string

	// Unmarshal, if set, will be called on the raw data returned from LUCI
	// Config.
	//
	// If unset:
	//   - If T is a concrete proto.Message, the config will be decoded as
	//     text-encoded proto with default options (namely, rejecting unknown
	//     fields).
	//   - If T is string or []byte, the raw content is passed through.
	//   - For any other type T, Unmarshal is required.
	Unmarshal UnmarshalFunc[T]
}

func (rf *RegexFile[T]) isRegex() bool { return true }

func (rf *RegexFile[T]) pattern() string { return rf.PathRegex }

func (rf *RegexFile[T]) recordInSnapshot(snap *Snapshot, path string, val any) {
	if val == nil {
		return
	}

	cur, _ := snap.regexFiles[rf].(map[string]T)
	if cur == nil {
		cur = map[string]T{}
		snap.regexFiles[rf] = cur
	}
	cur[path] = val.(T)
}

func (rf *RegexFile[T]) unmarshalFunc() (genericUnmarshalFunc, error) {
	return rf.Unmarshal.asGeneric()
}

// Get retrieves the map of (resolved file path -> unmarshaled value) for all
// files in snap matching rf.
//
// Returns a non-nil empty map if no files in the snapshot matched rf.
//
// Panics if rf was not registered in the pipeline that produced snap.
func (rf *RegexFile[T]) Get(snap *Snapshot) map[string]T {
	val, ok := snap.regexFiles[rf]
	if !ok {
		return nil
	}
	return val.(map[string]T)
}
