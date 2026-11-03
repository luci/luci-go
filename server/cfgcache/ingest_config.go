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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"iter"
	"maps"
	"regexp"
	"slices"
	"strings"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/data/text/pattern"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/validation"
	cfgcachepb "go.chromium.org/luci/server/cfgcache/proto"
)

type resolvedFile struct {
	File

	unmarshal genericUnmarshalFunc

	// Set in [Registry.Initialize]:
	path string          // rendered Path or PathRegex
	pat  pattern.Pattern // exact:<path> or regex:<rendered>
}

// ingestConfig is a type-erased version of [ServicePipeline] and
// [ProjectPipeline] used internally.
type ingestConfig struct {
	// Unique user-supplied pipeline ID.
	id string

	// Human-readable descriptor used in error messages.
	desc string

	// The 'pattern' for the ServiceName (if any) (e.g. this may have variables
	// like ${appid}).
	serviceNamePattern string

	// Files for this pipeline within a single config set.
	files []*resolvedFile

	// Original TransformVersion and a genericized version of TransformFunc.
	transformVersion int64
	transformFunc    genericTransformFunc
	store            genericStore

	// serviceName is the *resolved* serviceNamePattern. Set in
	// [Registry.Initialize].
	serviceName string
}

func (ic *ingestConfig) String() string {
	return ic.desc
}

// matchesPath returns true if this ingestConfig wants the config file `path`.
func (ic *ingestConfig) matchesPath(path string) bool {
	for _, rf := range ic.files {
		if rf.pat.Match(path) {
			return true
		}
	}
	return false
}

// multiFile returns true if this ingestConfig consumes more than one file
// from config.
func (ic *ingestConfig) multiFile() bool {
	if len(ic.files) > 1 {
		return true
	}
	for _, rf := range ic.files {
		if rf.isRegex() {
			return true
		}
	}
	return false
}

// compileFilesRegex returns a compiled regular expression that matches any file
// path matched by any of `ics`.
//
// Must be called after `rf.path` has been populated in [Registry.Initialize].
func compileFilesRegex(ics []*ingestConfig) *regexp.Regexp {
	var branches []string
	for _, ic := range ics {
		for _, rf := range ic.files {
			if rf.isRegex() {
				branches = append(branches, "(?:"+rf.path+")")
			} else {
				branches = append(branches, "(?:"+regexp.QuoteMeta(rf.path)+")")
			}
		}
	}
	slices.Sort(branches)
	branches = slices.Compact(branches)
	return regexp.MustCompile("^(?:" + strings.Join(branches, "|") + ")$")
}

// allDesiredMeta computes the Meta for the subset of snapshots matching ic.
//
// Does not populate [Meta.ContentSize].
func (ic *ingestConfig) allDesiredMeta(snaps loadedConfig) (ret map[config.Set]*cfgcachepb.Meta) {
	var configSets iter.Seq2[config.Set, flatSnapshot]
	if ic.serviceName != "" {
		configSets = snaps.serviceConfigs(ic.serviceName)
		ret = make(map[config.Set]*cfgcachepb.Meta, 1)
	} else {
		configSets = snaps.projectConfigs
		ret = make(map[config.Set]*cfgcachepb.Meta, len(snaps))
	}

	for cs, configs := range configSets {
		meta, ok := ic.desiredMeta(configs)
		if ok {
			ret[cs] = meta
		}
	}

	return ret
}

// desiredMeta computes the Meta for the subset of files in snap matching ic.
//
// Does not populate [Meta.ContentSize].
func (ic *ingestConfig) desiredMeta(snap flatSnapshot) (*cfgcachepb.Meta, bool) {
	h := sha256.New()
	var totalSize int64
	var revision string
	for _, cfg := range snap {
		if !ic.matchesPath(cfg.Path) {
			continue
		}
		if revision == "" {
			revision = cfg.Revision
		}
		fmt.Fprintf(h, "%s\x00%s\x00", cfg.Path, cfg.ContentHash)
		totalSize += cfg.ContentSize
	}

	if revision == "" {
		return nil, false
	}
	return &cfgcachepb.Meta{
		InputRevision:    revision,
		InputHash:        hex.EncodeToString(h.Sum(nil)),
		InputSize:        totalSize,
		TransformVersion: ic.transformVersion,
	}, true
}

// validateAndTransform unmarshals all files in rawFiles into a [Snapshot] and
// invokes the pipeline's TransformFunc.
//
// Returns the output of the TransformFunc (as proto.Message).
func (ic *ingestConfig) validateAndTransform(ctx *validation.Context, configSet config.Set, rawFiles map[string][]byte) (proto.Message, error) {
	snap := &Snapshot{
		configSet:   configSet,
		singleFiles: make(map[File]singleFileEntry),
		regexFiles:  make(map[File]any),
	}

	// Scan all files for SingleFiles; if we find any, initialize them with our
	// resolved path.
	//
	// We also default the validation context to the resolved name of the first
	// SingleFile in Files.
	var baseName string
	for _, file := range ic.files {
		if !file.isRegex() {
			if baseName == "" {
				baseName = file.path
			}
			file.recordInSnapshot(snap, file.path, nil)
		}
	}

	// Now, unmarshal everything.
	var hasUnmarshalErr bool
	for _, path := range slices.Sorted(maps.Keys(rawFiles)) {
		for _, file := range ic.files {
			if !file.pat.Match(path) {
				continue
			}
			ctx.SetFile(path)
			val, err := file.unmarshal(rawFiles[path])
			if err != nil {
				ctx.Errorf("failed to unmarshal: %s", err)
				hasUnmarshalErr = true
				continue
			}
			file.recordInSnapshot(snap, path, val)
		}
	}

	if hasUnmarshalErr {
		return nil, nil
	}

	ctx.SetFile(baseName)
	return ic.transformFunc(ctx, snap)
}
