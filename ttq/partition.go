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

// Package ttq implements transaction task queues on Google Cloud.
package ttq

import (
	"container/list"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"go.chromium.org/luci/common/errors"
)

type Partition struct {
	low  big.Int // inclusive
	high big.Int // exclusive. May be equal to max SHA2 hash value + 1.
}

func (p Partition) String() string {
	return fmt.Sprintf("%s_%s", p.low.Text(16), p.high.Text(16))
}

func PartitionFromString(s string) (p Partition, err error) {
	i := strings.Index(s, "_")
	if i == -1 {
		err = errors.Annotate(err, "partition %q has invalid format", s).Err()
		return
	}
	if err = setBigIntFromString(&p.low, s[:i]); err != nil {
		return
	}
	err = setBigIntFromString(&p.high, s[i+1:])
	return
}

func setBigIntFromString(b *big.Int, s string) error {
	if _, ok := b.SetString(s, 16); !ok {
		return errors.Reason("invalid bigint hex %q", s).Err()
	}
	return nil
}

func Universe(keySpaceBytes int) (p Partition) {
	p.high.SetBit(big.NewInt(0), keySpaceBytes*8, 1)
	return
}

func PartitionSpanning(sorted []*Reminder) *Partition {
	p := &Partition{}
	if len(sorted) == 0 {
		return p // empty partition of [0,0)
	}
	if err := setBigIntFromString(&p.low, sorted[0].ID); err != nil {
		panic(err)
	}
	if err := setBigIntFromString(&p.high, sorted[len(sorted)-1].ID); err != nil {
		panic(err)
	}
	p.high.Add(&p.high, bigInt1)
	return p
}

func paddedHex(b *big.Int, keySpaceBytes int) string {
	s := b.Text(16)
	return strings.Repeat("0", keySpaceBytes*2-len(s)) + s
}

func (p Partition) queryBounds(keySpaceBytes int) (low, high string) {
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
	remaining.low.Add(&remaining.low, bigInt1)
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
	// at the cost of readability.
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

func (p Partition) Copy() *Partition {
	r := &Partition{}
	r.low.Set(&p.low)
	r.high.Set(&p.high)
	return r
}

type SortedPartitions []*Partition

type SortedPartitionsBuilder struct {
	// should be a balanced tree for max performance, but for now list will suffice.
	l *list.List
}

func NewSortedPartitionsBuilder(p *Partition) SortedPartitionsBuilder {
	b := SortedPartitionsBuilder{l: list.New()}
	b.l.PushBack(p.Copy())
	return b
}

func (b *SortedPartitionsBuilder) IsEmpty() bool {
	return b.l.Len() == 0
}
func (b *SortedPartitionsBuilder) Result() SortedPartitions {
	r := make([]*Partition, 0, b.l.Len())
	for el := b.l.Front(); el != nil; el = el.Next() {
		r = append(r, el.Value.(*Partition))
	}
	return r
}

func (b *SortedPartitionsBuilder) Exclude(exclude *Partition) {
	for el := b.l.Front(); el != nil; {
		avail := el.Value.(*Partition)
		switch {
		case exclude.low.Cmp(&avail.high) >= 0:
			// avail < exclude
			el = el.Next()

		case exclude.high.Cmp(&avail.low) <= 0:
			// exclude < avail
			return

		case exclude.low.Cmp(&avail.low) <= 0:
			// front excluded
			if exclude.high.Cmp(&avail.high) >= 0 {
				// back also excluded
				next := el.Next()
				b.l.Remove(el)
				el = next
			} else {
				// only back remains.
				avail.low.Set(&exclude.high)
				return
			}

		case exclude.high.Cmp(&avail.high) >= 0:
			// only front remains.
			avail.high.Set(&exclude.low)
			el = el.Next()

		default:
			// middle is excluded.
			second := &Partition{}
			second.low.Set(&exclude.high)
			second.high.Set(&avail.high)
			avail.high.Set(&exclude.low)
			b.l.InsertAfter(second, el)
			return
		}
	}
}

func (ps SortedPartitions) filter(sorted []*Reminder, keySpaceBytes int) (res []*Reminder) {
	for len(ps) > 0 && len(sorted) > 0 {
		lowStr := paddedHex(&ps[0].low, keySpaceBytes)
		fr := sort.Search(len(sorted), func(i int) bool { return sorted[i].ID >= lowStr })
		if fr == len(sorted) {
			return
		}
		highStr := paddedHex(&ps[0].high, keySpaceBytes)
		to := sort.Search(len(sorted)-fr, func(i int) bool { return sorted[fr+i].ID >= highStr })
		if to > 0 {
			res = append(res, sorted[fr:fr+to]...)
		}
		// Can be optimized more by doing binary search over `ps` if fr == to == 0.
		sorted = sorted[fr+to:]
		ps = ps[1:]
	}
	return
}

var bigInt0 = big.NewInt(0)
var bigInt1 = big.NewInt(1)

func distance(low, high *big.Int) *big.Int {
	return (&big.Int{}).Sub(high, low)
}
