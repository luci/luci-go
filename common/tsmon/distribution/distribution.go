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

// Package distribution contains distribution metrics, fixed width and geometric
// bucketers.
package distribution

// A Distribution holds a statistical summary of a collection of floating-point
// values.
type Distribution struct {
	b *Bucketer

	buckets           []int64
	count             int64
	sum               float64
	lastNonZeroBucket int
}

// New creates a new distribution using the given bucketer.  Passing a nil
// Bucketer will use DefaultBucketer.
func New(b *Bucketer) *Distribution {
	if b == nil {
		b = DefaultBucketer
	}
	return &Distribution{b: b, lastNonZeroBucket: -1}
}

// Add adds the sample to the distribution and updates the statistics.
func (d *Distribution) Add(sample float64) {
	i := d.b.Bucket(sample)
	if i >= len(d.buckets) {
		d.buckets = append(d.buckets, make([]int64, i-len(d.buckets)+1)...)
	}
	d.buckets[i]++
	d.sum += sample
	d.count++
	if i > d.lastNonZeroBucket {
		d.lastNonZeroBucket = i
	}
}

// Bucketer returns the bucketer used in this distribution.
func (d *Distribution) Bucketer() *Bucketer { return d.b }

// Buckets provides access to the underlying buckets slice.  len(Buckets) will
// be <= Bucketer().NumBuckets()
func (d *Distribution) Buckets() []int64 { return d.buckets }

// Count returns the number of times Add has been called.
func (d *Distribution) Count() int64 { return d.count }

// Sum returns the sum of all samples passed to Add.
func (d *Distribution) Sum() float64 { return d.sum }

// LastNonZeroBucket returns the index into Buckets() of the last bucket that
// is set (non-zero).  Returns -1 if Count() == 0.
func (d *Distribution) LastNonZeroBucket() int { return d.lastNonZeroBucket }
