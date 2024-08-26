// Copyright 2015 The LUCI Authors.
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

package distribution

import (
	"fmt"
	"math"
	"sort"
)

// A Bucketer maps samples into discrete buckets.
type Bucketer struct {
	width, growthFactor float64
	numFiniteBuckets    int
	lowerBounds         []float64
	scale               float64
}

// NewBucketer creates a bucketer from custom parameters.
func NewBucketer(width, growthFactor float64, numFiniteBuckets int, scale float64) *Bucketer {
	b := &Bucketer{
		width:            width,
		growthFactor:     growthFactor,
		numFiniteBuckets: numFiniteBuckets,
		scale:            scale,
	}
	b.init()
	return b
}

// FixedWidthBucketer returns a Bucketer that uses numFiniteBuckets+2 buckets:
//
//	bucket[0] covers (-Inf...0)
//	bucket[i] covers [width*(i-1)...width*i) for i > 0 and i <= numFiniteBuckets
//	bucket[numFiniteBuckets+1] covers [width*numFiniteBuckets...+Inf)
//
// The width must be greater than 0, and the number of finite buckets must be
// greater than 0.
func FixedWidthBucketer(width float64, numFiniteBuckets int) *Bucketer {
	return NewBucketer(width, 0, numFiniteBuckets, 1.0)
}

// GeometricBucketer returns a Bucketer that uses numFiniteBuckets+2 buckets:
//
//	bucket[0] covers (−Inf, 1)
//	bucket[i] covers [growthFactor^(i−1), growthFactor^i) for i > 0 and i <= numFiniteBuckets
//	bucket[numFiniteBuckets+1] covers [growthFactor^(numFiniteBuckets−1), +Inf)
//
// growthFactor must be greater than 1.0, and the number of finite buckets must
// be greater than 0.
func GeometricBucketer(growthFactor float64, numFiniteBuckets int) *Bucketer {
	return NewBucketer(0, growthFactor, numFiniteBuckets, 1.0)
}

// GeometricBucketerWithScale returns a Bucketer that uses numFiniteBuckets+2 buckets:
//
//	bucket[0] covers (−Inf, scale)
//	bucket[i] covers [scale*growthFactor^(i−1), scale*growthFactor^i) for i > 0 and i <= numFiniteBuckets
//	bucket[numFiniteBuckets+1] covers [scale*growthFactor^(numFiniteBuckets−1), +Inf)
//
// growthFactor must be greater than 1.0, the number of finite buckets and scale
// must be greater than 0.
func GeometricBucketerWithScale(growthFactor float64, numFiniteBuckets int, scale float64) *Bucketer {
	return NewBucketer(0, growthFactor, numFiniteBuckets, scale)
}

// DefaultBucketer is a bucketer with sensible bucket sizes.
var DefaultBucketer = GeometricBucketer(math.Pow(10, 0.2), 100)

// NumFiniteBuckets returns the number of finite buckets.
func (b *Bucketer) NumFiniteBuckets() int { return b.numFiniteBuckets }

// NumBuckets returns the number of buckets including the underflow and overflow
// buckets.
func (b *Bucketer) NumBuckets() int { return b.numFiniteBuckets + 2 }

// Width returns the bucket width used to configure this Bucketer.
func (b *Bucketer) Width() float64 { return b.width }

// GrowthFactor returns the growth factor used to configure this Bucketer.
func (b *Bucketer) GrowthFactor() float64 { return b.growthFactor }

// Scale returns the scale used to configure this Bucketer.
func (b *Bucketer) Scale() float64 { return b.scale }

// UnderflowBucket returns the index of the underflow bucket.
func (b *Bucketer) UnderflowBucket() int { return 0 }

// OverflowBucket returns the index of the overflow bucket.
func (b *Bucketer) OverflowBucket() int { return b.numFiniteBuckets + 1 }

// Bucket returns the index of the bucket for sample.
// TODO(dsansome): consider reimplementing sort.Search inline to avoid overhead
// of calling a function to compare two values.
func (b *Bucketer) Bucket(sample float64) int {
	return sort.Search(b.NumBuckets(), func(i int) bool { return sample < b.lowerBounds[i] }) - 1
}

func (b *Bucketer) init() {
	if b.numFiniteBuckets <= 0 {
		panic(fmt.Sprintf("numFiniteBuckets must be positive (was %d)", b.numFiniteBuckets))
	}

	if b.scale <= 0 {
		panic(fmt.Sprintf("scale must be positive (was %f)", b.scale))
	}

	b.lowerBounds = make([]float64, b.NumBuckets())
	b.lowerBounds[0] = math.Inf(-1)

	if b.width != 0 {
		b.fillLinearBounds()
	} else {
		b.fillExponentialBounds()
	}

	// Sanity check that the bucket boundaries are monotonically increasing.
	var previous float64
	for i, bound := range b.lowerBounds {
		if i != 0 && bound <= previous {
			panic("bucket boundaries must be monotonically increasing")
		}
		previous = bound
	}
}

func (b *Bucketer) fillLinearBounds() {
	for i := 1; i < b.NumBuckets(); i++ {
		b.lowerBounds[i] = b.width * float64(i-1)
	}
}

func (b *Bucketer) fillExponentialBounds() {
	for i := 1; i < b.NumBuckets(); i++ {
		b.lowerBounds[i] = b.scale * math.Pow(b.growthFactor, float64(i-1))
	}
}
