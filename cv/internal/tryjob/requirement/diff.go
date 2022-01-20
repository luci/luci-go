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

// DiffResult reports the diff between two tryjob requirements
type DiffResult struct {
	ExtraDefs          []*tryjob.Definition
	RemovedDefs        []*tryjob.Definition
	RetryConfigChanged bool
}

// Diff computes the diff between two tryjob requirements.
func Diff(base, target *tryjob.Requirement) DiffResult {
	sortedBaseDefs := toSortedTryjobDefs(base.GetDefinitions())
	sortedTargetDefs := toSortedTryjobDefs(target.GetDefinitions())

	res := DiffResult{}
	for len(sortedBaseDefs) > 0 && len(sortedTargetDefs) > 0 {
		switch bytes.Compare(sortedBaseDefs[0].sortKey, sortedTargetDefs[0].sortKey) {
		case 0:
			sortedBaseDefs = sortedBaseDefs[1:]
			sortedTargetDefs = sortedTargetDefs[1:]
		case -1:
			res.RemovedDefs = append(res.RemovedDefs, sortedBaseDefs[0].def)
			sortedBaseDefs = sortedBaseDefs[1:]
		case 1:
			res.ExtraDefs = append(res.ExtraDefs, sortedTargetDefs[0].def)
			sortedTargetDefs = sortedTargetDefs[1:]
		}
	}
	// add the remaining definitions
	for _, def := range sortedBaseDefs {
		res.RemovedDefs = append(res.RemovedDefs, def.def)
	}
	for _, def := range sortedTargetDefs {
		res.ExtraDefs = append(res.ExtraDefs, def.def)
	}

	if !proto.Equal(base.GetRetryConfig(), target.GetRetryConfig()) {
		res.RetryConfigChanged = true
	}
	return res
}

type comparableTryjobDef struct {
	def *tryjob.Definition
	// sortKey is comparable binary encoding of `def`
	sortKey []byte
}

func toSortedTryjobDefs(defs []*tryjob.Definition) []comparableTryjobDef {
	if len(defs) == 0 {
		return nil // optimization: avoid initialize empty slice
	}
	ret := make([]comparableTryjobDef, len(defs))
	for i, def := range defs {
		if def.GetBuildbucket() == nil {
			panic(fmt.Errorf("only support buildbucket backend; got %T", def.GetBackend()))
		}
		buf := &bytes.Buffer{}
		encodeBuildbucketDef(buf, def.GetBuildbucket())
		// If this tryjob definition has equivalent tryjob defined, append the
		// encoded equivalent tryjob definition to the buffer.
		if equiDef := def.GetEquivalentTo(); equiDef != nil {
			if equiDef.GetBuildbucket() == nil {
				panic(fmt.Errorf("only support buildbucket backend for equivalent tryjob; got %T", equiDef.GetBackend()))
			}
			encodeBuildbucketDef(buf, equiDef.GetBuildbucket())
		}
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
