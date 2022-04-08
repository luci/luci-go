// Copyright 2022 The LUCI Authors.
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

package requirement

import (
	"bytes"
	"fmt"
	"sort"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/data/cmpbin"

	"go.chromium.org/luci/cv/internal/tryjob"
)

// DefinitionMapping is a mapping between two definitions.
type DefinitionMapping map[*tryjob.Definition]*tryjob.Definition

// Has reports whether the given tryjob definition is in the mapping as a key.
func (dm DefinitionMapping) Has(def *tryjob.Definition) bool {
	_, ok := dm[def]
	return ok
}

// DiffResult contains a diff between two Tryjob requirements.
type DiffResult struct {
	// RemovedDefs contains the definitions in `base` that are no longer present
	// in `target`.
	//
	// Only the keys will be populated, the values will be nil.
	RemovedDefs DefinitionMapping
	// ChangedDefs contains mapping between definitions in `base` and `target`
	// that have changed.
	//
	// The categorization of changes may vary across different backend. For
	// buildbucket tryjob, a definition is considered changed if the main builder
	// stays the same and other properties like equivalent builder, reuse config
	// have changed.
	ChangedDefs DefinitionMapping
	// AddedDefs contains the definitions that are added to `target` but are not
	// in `base`.
	//
	// Only the keys will be populated, the values will be nil.
	AddedDefs DefinitionMapping
	// RetryConfigChanged indicates the retry configuration has changed.
	RetryConfigChanged bool
}

// Diff computes the diff between two Tryjob Requirements.
func Diff(base, target *tryjob.Requirement) DiffResult {
	sortedBaseDefs := toSortedTryjobDefs(base.GetDefinitions())
	sortedTargetDefs := toSortedTryjobDefs(target.GetDefinitions())

	res := DiffResult{
		AddedDefs:   make(DefinitionMapping),
		ChangedDefs: make(DefinitionMapping),
		RemovedDefs: make(DefinitionMapping),
	}
	for len(sortedBaseDefs) > 0 && len(sortedTargetDefs) > 0 {
		baseDef, targetDef := sortedBaseDefs[0].def, sortedTargetDefs[0].def
		switch bytes.Compare(sortedBaseDefs[0].sortKey, sortedTargetDefs[0].sortKey) {
		case 0:
			if !proto.Equal(baseDef, targetDef) {
				res.ChangedDefs[baseDef] = targetDef
			}
			sortedBaseDefs, sortedTargetDefs = sortedBaseDefs[1:], sortedTargetDefs[1:]
		case -1:
			// The head of the sortedBaseDefs is lower than the head of ,
			// sortedTargetDefs, this implies that the definition at the head of
			// sortedBaseDefs has been removed.
			res.RemovedDefs[baseDef] = nil
			sortedBaseDefs = sortedBaseDefs[1:]
		case 1:
			// Converse case of the above.
			res.AddedDefs[targetDef] = nil
			sortedTargetDefs = sortedTargetDefs[1:]
		}
	}
	// Add the remaining definitions.
	for _, def := range sortedBaseDefs {
		res.RemovedDefs[def.def] = nil
	}
	for _, def := range sortedTargetDefs {
		res.AddedDefs[def.def] = nil
	}

	if !proto.Equal(base.GetRetryConfig(), target.GetRetryConfig()) {
		res.RetryConfigChanged = true
	}
	return res
}

type comparableTryjobDef struct {
	def *tryjob.Definition
	// sortKey is comparable binary encoding of critical part of definition.
	//
	// For example, for Buildbucket tryjob, the key will be encode(host
	// + fully qualified builder name).
	sortKey []byte
}

func toSortedTryjobDefs(defs []*tryjob.Definition) []comparableTryjobDef {
	if len(defs) == 0 {
		return nil // Optimization: avoid initialize empty slice.
	}
	ret := make([]comparableTryjobDef, len(defs))
	for i, def := range defs {
		if def.GetBuildbucket() == nil {
			panic(fmt.Errorf("only support buildbucket backend; got %T", def.GetBackend()))
		}
		buf := &bytes.Buffer{}
		encodeBuildbucketDef(buf, def.GetBuildbucket())
		ret[i] = comparableTryjobDef{
			def:     def,
			sortKey: buf.Bytes(),
		}
	}
	sort.Slice(ret, func(i, j int) bool {
		return bytes.Compare(ret[i].sortKey, ret[j].sortKey) < 0
	})
	return ret
}

func encodeBuildbucketDef(buf cmpbin.WriteableBytesBuffer, bbdef *tryjob.Definition_Buildbucket) {
	mustCmpbinWriteString(buf, bbdef.GetHost())
	builder := bbdef.GetBuilder()
	mustCmpbinWriteString(buf, builder.GetProject())
	mustCmpbinWriteString(buf, builder.GetBucket())
	mustCmpbinWriteString(buf, builder.GetBuilder())
}

func mustCmpbinWriteString(buf cmpbin.WriteableBytesBuffer, str string) {
	_, err := cmpbin.WriteString(buf, str)
	if err != nil {
		panic(err)
	}
}
