// Copyright 2020 The LUCI Authors.
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

// Package partition encapsulates partioning and querying large keyspace which
// can't be expressed even as as uint64.
//
// All to/from string functions use hex encoding.
package partition

import (
	"fmt"
	"math/big"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// Partition represents a range [Low..High).
type Partition struct {
	low  big.Int // inclusive
	high big.Int // exclusive. May be equal to max SHA2 hash value + 1.
}

// SortedPartitions are disjoint partitions sorted by ascending .Low field.
type SortedPartitions []*Partition

func FromInts(low, high int64) *Partition {
	p := &Partition{}
	p.low.SetInt64(low)
	p.high.SetInt64(high)
	return p
}

func SpanInclusive(low, highInclusive string) (*Partition, error) {
	p := &Partition{}
	if err := setBigIntFromString(&p.low, low); err != nil {
		return nil, err
	}
	if err := setBigIntFromString(&p.high, highInclusive); err != nil {
		return nil, err
	}
	p.high.Add(&p.high, bigInt1) // s.high++
	return p, nil
}

func Universe(keySpaceBytes int) *Partition {
	p := &Partition{}
	p.high.SetBit(&p.high, keySpaceBytes*8, 1) // 2^(keySpaceBytes*8)
	return p
}

func FromString(s string) (*Partition, error) {
	i := strings.Index(s, "_")
	if i == -1 {
		return nil, errors.Reason("partition %q has invalid format", s).Err()
	}
	p := &Partition{}
	if err := setBigIntFromString(&p.low, s[:i]); err != nil {
		return nil, err
	}
	if err := setBigIntFromString(&p.high, s[i+1:]); err != nil {
		return nil, err
	}
	return p, nil
}

func (p Partition) String() string {
	return fmt.Sprintf("%s_%s", p.low.Text(16 /*hex*/), p.high.Text(16 /*hex*/))
}

func (p Partition) Copy() *Partition {
	r := &Partition{}
	r.low.Set(&p.low)
	r.high.Set(&p.high)
	return r
}

func (p Partition) QueryBounds(keySpaceBytes int) (low, high string) {
	low = paddedHex(&p.low, keySpaceBytes)
	keySpaceHigh := new(big.Int)
	keySpaceHigh.SetBit(keySpaceHigh, keySpaceBytes*8, 1)
	if p.high.Cmp(keySpaceHigh) >= 0 {
		high = "g" // all hex strings are smaller than "g".
	} else {
		high = paddedHex(&p.high, keySpaceBytes)
	}
	return
}

func (p Partition) Split(shards int) SortedPartitions {
	if shards <= 0 {
		panic(">=1 shard required")
	}
	var increment, remainder, cur big.Int
	increment.QuoRem(
		cur.Sub(&p.high, &p.low),
		big.NewInt(int64(shards)),
		&remainder)
	if remainder.Cmp(bigInt0) > 0 {
		increment.Add(&increment, bigInt1)
	}

	partitions := make([]*Partition, 0, shards)
	cur.Set(&p.low)
	for cur.Cmp(&p.high) < 0 {
		next := &Partition{}
		next.low.Set(&cur)
		next.high.Add(&cur, &increment)
		cur.Set(&next.high)
		partitions = append(partitions, next)
	}
	// Due to int division to compute the increment, need to ensure last partition
	// covers exactly to the end of the orignal.
	partitions[len(partitions)-1].high = p.high
	return partitions
}

// EducatedSplitAfter splits partition after a given boundary assuming constant
// density s.t. each shard has approximately targetItems.
//
// Caps the number of resulting partitions to at most maxShards.
// panics if called on invalid data.
func (p Partition) EducatedSplitAfter(exclusive string, beforeItems, targetItems, maxShards int) SortedPartitions {
	remaining := Partition{}
	if err := setBigIntFromString(&remaining.low, exclusive); err != nil {
		panic(err)
	}
	if p.low.Cmp(&remaining.low) > 0 { // low > cur
		panic("must be within the partition")
	}
	if p.high.Cmp(&remaining.high) <= 0 { // high <= cur
		panic("must be within the partition")
	}
	remaining.low.Add(&remaining.low, bigInt1) // low++
	remaining.high.Set(&p.high)

	// Compute expShards as
	//
	//	   beforeItems / len(before) * len(remaining) / targetItems
	//
	// in a somewhat readable way as
	//
	//	   (beforeItems * len(remaining)) / ( targetItems * len(before))
	//
	// NOTE: this can be optimized if needed to avoid excessive memory allocations
	// in bit.Int at the cost of readability.
	iBefore := big.NewInt(int64(beforeItems))
	iTarget := big.NewInt(int64(targetItems))
	var expShards, iRemainder big.Int
	expShards.QuoRem(
		(&big.Int{}).Mul(iBefore, distance(&remaining.low, &remaining.high)),
		(&big.Int{}).Mul(iTarget, distance(&p.low, &remaining.low)),
		&iRemainder,
	)
	if iRemainder.Cmp(bigInt0) > 0 {
		expShards.Add(&expShards, bigInt1)
	}
	shards := maxShards
	if expShards.Cmp(big.NewInt(int64(maxShards))) < 0 {
		shards = int(expShards.Int64())
	}
	return remaining.Split(shards)
}

var (
	// these are effectively constants predefined to avoid needless memory allocations.

	bigInt0 = big.NewInt(0)
	bigInt1 = big.NewInt(1)
)

func distance(low, high *big.Int) *big.Int {
	return (&big.Int{}).Sub(high, low)
}

func setBigIntFromString(b *big.Int, s string) error {
	if _, ok := b.SetString(s, 16 /*hex*/); !ok {
		return errors.Reason("invalid bigint hex %q", s).Err()
	}
	return nil
}

func paddedHex(b *big.Int, keySpaceBytes int) string {
	s := b.Text(16 /*hex*/)
	return strings.Repeat("0", keySpaceBytes*2-len(s)) + s
}
