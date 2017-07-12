// Copyright 2016 The LUCI Authors.
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

package jobsim

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
)

// ToSlice returns this SparseRange as an expanded slice of uint32s.
func (s *SparseRange) ToSlice() (ret []uint32) {
	for _, itm := range s.Items {
		switch x := itm.RangeItem.(type) {
		case *RangeItem_Single:
			ret = append(ret, x.Single)
		case *RangeItem_Range:
			for i := x.Range.Low; i <= x.Range.High; i++ {
				ret = append(ret, i)
			}
		}
	}
	return
}

// ExpandShards expands any dependencies that have non-zero Shards values.
func (s *DepsStage) ExpandShards() {
	newSlice := make([]*Dependency, 0, len(s.Deps))
	for _, dep := range s.Deps {
		if dep.Shards != 0 {
			for i := uint64(0); i < dep.Shards; i++ {
				depCopy := *dep
				depCopy.Shards = 0
				phraseCopy := *depCopy.Phrase
				phraseCopy.Name = fmt.Sprintf("%s_%d", phraseCopy.Name, i)
				depCopy.Phrase = &phraseCopy
				newSlice = append(newSlice, &depCopy)
			}
		} else {
			newSlice = append(newSlice, dep)
		}
	}
	s.Deps = newSlice
}

// fastHash is a non-cryptographic NxN -> N hash function. It's used to
// deterministically blend seeds for subjobs.
func fastHash(a int64, bs ...int64) int64 {
	buf := make([]byte, 8)
	hasher := fnv.New64a()
	w := func(v int64) {
		binary.LittleEndian.PutUint64(buf, uint64(v))
		if _, err := hasher.Write(buf); err != nil {
			panic(err)
		}
	}
	w(a)
	for _, b := range bs {
		w(b)
	}
	return int64(hasher.Sum64())
}

// Seed rewrites this dependency's Seed value
func (d *Dependency) Seed(rnd *rand.Rand, seed, round int64) {
	if d.MixSeed {
		if d.Phrase.Seed == 0 {
			d.Phrase.Seed = rnd.Int63()
		} else {
			d.Phrase.Seed = fastHash(seed*(math.MaxInt32), d.Phrase.Seed, round)
		}
	} else if d.Phrase.Seed == 0 {
		d.Phrase.Seed = seed
	}
}
