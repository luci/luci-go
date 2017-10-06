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

package memory

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/datastore/serialize"
	"go.chromium.org/luci/common/data/stringset"
)

// ErrMissingIndex is returned when the current indexes are not sufficient
// for the current query.
type ErrMissingIndex struct {
	ns      string
	Missing *ds.IndexDefinition
}

func (e *ErrMissingIndex) Error() string {
	yaml, err := e.Missing.YAMLString()
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf(
		"Insufficient indexes. Consider adding:\n%s", yaml)
}

// reducedQuery contains only the pieces of the query necessary to iterate for
// results.
//   deduplication is applied externally
//   projection / keysonly / entity retrieval is done externally
type reducedQuery struct {
	kc   ds.KeyContext
	kind string

	// eqFilters indicate the set of all prefix constraints which need to be
	// fulfilled in the composite query. All of these will translate into prefix
	// bytes for SOME index.
	eqFilters map[string]stringset.Set

	// suffixFormat is the PRECISE listing of the suffix columns that ALL indexes
	//   in the multi query will have.
	//
	// suffixFormat ALWAYS includes the inequality filter (if any) as the 0th
	//   element
	// suffixFormat ALWAYS includes any additional projections (in ascending
	//   order) after all user defined sort orders
	// suffixFormat ALWAYS has __key__ as the last column
	suffixFormat []ds.IndexColumn

	// limits of the inequality and/or full sort order. This is ONLY a suffix,
	// and it will be appended to the prefix during iteration.
	start []byte
	end   []byte

	// metadata describing the total number of columns that this query requires to
	// execute perfectly.
	numCols int
}

type indexDefinitionSortable struct {
	// eqFilts is the list of ACTUAL prefix columns. Note that it may contain
	// redundant columns! (e.g. (tag, tag) is a perfectly valid prefix, becuase
	// (tag=1, tag=2) is a perfectly valid query).
	eqFilts []ds.IndexColumn
	coll    memCollection
}

func (i *indexDefinitionSortable) hasAncestor() bool {
	return len(i.eqFilts) > 0 && i.eqFilts[0].Property == "__ancestor__"
}

func (i *indexDefinitionSortable) numEqHits(c *constraints) int {
	ret := 0
	for _, filt := range i.eqFilts {
		if _, ok := c.constraints[filt.Property]; ok {
			ret++
		}
	}
	return ret
}

type indexDefinitionSortableSlice []indexDefinitionSortable

func (idxs indexDefinitionSortableSlice) Len() int      { return len(idxs) }
func (idxs indexDefinitionSortableSlice) Swap(i, j int) { idxs[i], idxs[j] = idxs[j], idxs[i] }
func (idxs indexDefinitionSortableSlice) Less(i, j int) bool {
	a, b := idxs[i], idxs[j]
	if a.coll == nil && b.coll != nil {
		return true
	} else if a.coll != nil && b.coll == nil {
		return false
	}

	cmp := len(a.eqFilts) - len(b.eqFilts)
	if cmp < 0 {
		return true
	} else if cmp > 0 {
		return false
	}
	for k, col := range a.eqFilts {
		ocol := b.eqFilts[k]
		if !col.Descending && ocol.Descending {
			return true
		} else if col.Descending && !ocol.Descending {
			return false
		}
		if col.Property < ocol.Property {
			return true
		} else if col.Property > ocol.Property {
			return false
		}
	}
	return false
}

// maybeAddDefinition possibly adds a new indexDefinitionSortable to this slice.
// It's only added if it could be useful in servicing q, otherwise this function
// is a noop.
//
// This returns true iff the proposed index is OK and depletes missingTerms to
// empty.
//
// If the proposed index is PERFECT (e.g. contains enough columns to cover all
// equality filters, and also has the correct suffix), idxs will be replaced
// with JUST that index, and this will return true.
func (idxs *indexDefinitionSortableSlice) maybeAddDefinition(q *reducedQuery, s memStore, missingTerms stringset.Set, id *ds.IndexDefinition) bool {
	// Kindless queries are handled elsewhere.
	if id.Kind != q.kind {
		impossible(
			fmt.Errorf("maybeAddDefinition given index with wrong kind %q v %q", id.Kind, q.kind))
	}

	// If we're an ancestor query, and the index is compound, but doesn't include
	// an Ancestor field, it doesn't work. Builtin indexes can be used for
	// ancestor queries (and have !Ancestor), assuming that it's only equality
	// filters (plus inequality on __key__), or a single inequality.
	if q.eqFilters["__ancestor__"] != nil && !id.Ancestor && !id.Builtin() {
		impossible(
			fmt.Errorf("maybeAddDefinition given compound index with wrong ancestor info: %s %#v", id, q))
	}

	// add __ancestor__ if necessary
	sortBy := id.GetFullSortOrder()

	// If the index has fewer fields than we need for the suffix, it can't
	// possibly help.
	if len(sortBy) < len(q.suffixFormat) {
		return false
	}

	numEqFilts := len(sortBy) - len(q.suffixFormat)
	// make sure the orders are precisely the same
	for i, sb := range sortBy[numEqFilts:] {
		if q.suffixFormat[i] != sb {
			return false
		}
	}

	if id.Builtin() && numEqFilts == 0 {
		if len(q.eqFilters) > 1 || (len(q.eqFilters) == 1 && q.eqFilters["__ancestor__"] == nil) {
			return false
		}
		if len(sortBy) > 1 && q.eqFilters["__ancestor__"] != nil {
			return false
		}
	}

	// Make sure the equalities section doesn't contain any properties we don't
	// want in our query.
	//
	// numByProp && totalEqFilts will be used to see if this is a perfect match
	// later.
	numByProp := make(map[string]int, len(q.eqFilters))
	totalEqFilts := 0

	eqFilts := sortBy[:numEqFilts]
	for _, p := range eqFilts {
		if _, ok := q.eqFilters[p.Property]; !ok {
			return false
		}
		numByProp[p.Property]++
		totalEqFilts++
	}

	// ok, we can actually use this

	// Grab the collection for convenience later. We don't want to invalidate this
	// index's potential just because the collection doesn't exist. If it's
	// a builtin and it doesn't exist, it still needs to be one of the 'possible'
	// indexes... it just means that the user's query will end up with no results.
	coll := s.GetCollection(
		fmt.Sprintf("idx:%s:%s", q.kc.Namespace, serialize.ToBytes(*id.PrepForIdxTable())))

	// First, see if it's a perfect match. If it is, then our search is over.
	//
	// A perfect match contains ALL the equality filter columns (or more, since
	// we can use residuals to fill in the extras).
	for _, sb := range eqFilts {
		missingTerms.Del(sb.Property)
	}

	perfect := false
	if len(sortBy) == q.numCols {
		perfect = true
		for k, num := range numByProp {
			if num < q.eqFilters[k].Len() {
				perfect = false
				break
			}
		}
	}
	toAdd := indexDefinitionSortable{coll: coll, eqFilts: eqFilts}
	if perfect {
		*idxs = indexDefinitionSortableSlice{toAdd}
	} else {
		*idxs = append(*idxs, toAdd)
	}
	return missingTerms.Len() == 0
}

// getRelevantIndexes retrieves the relevant indexes which could be used to
// service q. It returns nil if it's not possible to service q with the current
// indexes.
func getRelevantIndexes(q *reducedQuery, s memStore) (indexDefinitionSortableSlice, error) {
	missingTerms := stringset.New(len(q.eqFilters))
	for k := range q.eqFilters {
		if k == "__ancestor__" {
			// ancestor is not a prefix which can be satisfied by a single index. It
			// must be satisfied by ALL indexes (and has special logic for this in
			// the addDefinition logic)
			continue
		}
		missingTerms.Add(k)
	}
	idxs := indexDefinitionSortableSlice{}

	// First we add builtins
	// add
	//   idx:KIND
	if idxs.maybeAddDefinition(q, s, missingTerms, &ds.IndexDefinition{
		Kind: q.kind,
	}) {
		return idxs, nil
	}

	// add
	//   idx:KIND:prop
	//   idx:KIND:-prop
	props := stringset.New(len(q.eqFilters) + len(q.suffixFormat))
	for prop := range q.eqFilters {
		props.Add(prop)
	}
	for _, col := range q.suffixFormat[:len(q.suffixFormat)-1] {
		props.Add(col.Property)
	}
	for _, prop := range props.ToSlice() {
		if strings.HasPrefix(prop, "__") && strings.HasSuffix(prop, "__") {
			continue
		}
		if idxs.maybeAddDefinition(q, s, missingTerms, &ds.IndexDefinition{
			Kind: q.kind,
			SortBy: []ds.IndexColumn{
				{Property: prop},
			},
		}) {
			return idxs, nil
		}
		if idxs.maybeAddDefinition(q, s, missingTerms, &ds.IndexDefinition{
			Kind: q.kind,
			SortBy: []ds.IndexColumn{
				{Property: prop, Descending: true},
			},
		}) {
			return idxs, nil
		}
	}

	// Try adding all compound indexes whose suffix matches.
	suffix := &ds.IndexDefinition{
		Kind:     q.kind,
		Ancestor: q.eqFilters["__ancestor__"] != nil,
		SortBy:   q.suffixFormat,
	}
	walkCompIdxs(s, suffix, func(def *ds.IndexDefinition) bool {
		// keep walking until we find a perfect index.
		return !idxs.maybeAddDefinition(q, s, missingTerms, def)
	})

	// this query is impossible to fulfil with the current indexes. Not all the
	// terms (equality + projection) are satisfied.
	if missingTerms.Len() > 0 || len(idxs) == 0 {
		remains := &ds.IndexDefinition{
			Kind:     q.kind,
			Ancestor: q.eqFilters["__ancestor__"] != nil,
		}
		terms := missingTerms.ToSlice()
		if serializationDeterministic {
			sort.Strings(terms)
		}
		for _, term := range terms {
			remains.SortBy = append(remains.SortBy, ds.IndexColumn{Property: term})
		}
		remains.SortBy = append(remains.SortBy, q.suffixFormat...)
		last := remains.SortBy[len(remains.SortBy)-1]
		if !last.Descending {
			// this removes the __key__ column, since it's implicit.
			remains.SortBy = remains.SortBy[:len(remains.SortBy)-1]
		}
		if remains.Builtin() {
			impossible(
				fmt.Errorf("recommended missing index would be a builtin: %s", remains))
		}
		return nil, &ErrMissingIndex{q.kc.Namespace, remains}
	}

	return idxs, nil
}

// generate generates a single iterDefinition for the given index.
func generate(q *reducedQuery, idx *indexDefinitionSortable, c *constraints) *iterDefinition {
	def := &iterDefinition{
		c:     idx.coll,
		start: q.start,
		end:   q.end,
	}
	toJoin := make([][]byte, len(idx.eqFilts))
	for _, sb := range idx.eqFilts {
		val := c.peel(sb.Property)
		if sb.Descending {
			val = serialize.Invert(val)
		}
		toJoin = append(toJoin, val)
	}
	def.prefix = serialize.Join(toJoin...)
	def.prefixLen = len(def.prefix)

	if q.eqFilters["__ancestor__"] != nil && !idx.hasAncestor() {
		// The query requires an ancestor, but the index doesn't explicitly have it
		// as part of the prefix (otherwise it would have been the first eqFilt
		// above). This happens when it's a builtin index, or if it's the primary
		// index (for a kindless query), or if it's the Kind index (for a filterless
		// query).
		//
		// builtin indexes are:
		//   Kind/__key__
		//   Kind/Prop/__key__
		//   Kind/Prop/-__key__
		if len(q.suffixFormat) > 2 || q.suffixFormat[len(q.suffixFormat)-1].Property != "__key__" {
			// This should never happen. One of the previous validators would have
			// selected a different index. But just in case.
			impossible(fmt.Errorf("cannot supply an implicit ancestor for %#v", idx))
		}

		// get the only value out of __ancestor__
		anc, _ := q.eqFilters["__ancestor__"].Peek()

		// Intentionally do NOT update prefixLen. This allows multiIterator to
		// correctly include the entire key in the shared iterator suffix, instead
		// of just the remainder.

		// chop the terminal null byte off the q.ancestor key... we can accept
		// anything which is a descendant or an exact match.  Removing the last byte
		// from the key (the terminating null) allows this trick to work. Otherwise
		// it would be a closed range of EXACTLY this key.
		chopped := []byte(anc[:len(anc)-1])
		if q.suffixFormat[0].Descending {
			chopped = serialize.Invert(chopped)
		}
		def.prefix = serialize.Join(def.prefix, chopped)

		// Update start and end, since we know that if they contain anything, they
		// contain values for the __key__ field. This is necessary because bytes
		// are shifting from the suffix to the prefix, and start/end should only
		// contain suffix (variable) bytes.
		if def.start != nil {
			if !bytes.HasPrefix(def.start, chopped) {
				// again, shouldn't happen, but if it does, we want to know about it.
				impossible(fmt.Errorf(
					"start suffix for implied ancestor doesn't start with ancestor! start:%v ancestor:%v",
					def.start, chopped))
			}
			def.start = def.start[len(chopped):]
		}
		if def.end != nil {
			if !bytes.HasPrefix(def.end, chopped) {
				impossible(fmt.Errorf(
					"end suffix for implied ancestor doesn't start with ancestor! end:%v ancestor:%v",
					def.end, chopped))
			}
			def.end = def.end[len(chopped):]
		}
	}

	return def
}

type constraints struct {
	constraints     map[string][][]byte
	original        map[string][][]byte
	residualMapping map[string]int
}

// peel picks a constraint value for the property. It then removes this value
// from constraints (possibly removing the entire row from constraints if it
// was the last value). If the value wasn't available in constraints, it picks
// the value from residuals.
func (c *constraints) peel(prop string) []byte {
	ret := []byte(nil)
	if vals, ok := c.constraints[prop]; ok {
		ret = vals[0]
		if len(vals) == 1 {
			delete(c.constraints, prop)
		} else {
			c.constraints[prop] = vals[1:]
		}
	} else {
		row := c.original[prop]
		idx := c.residualMapping[prop]
		c.residualMapping[prop]++
		ret = row[idx%len(row)]
	}
	return ret
}

func (c *constraints) empty() bool {
	return len(c.constraints) == 0
}

// calculateConstraints produces a mapping of all equality filters to the values
// that they're constrained to. It also calculates residuals, which are an
// arbitrary value for filling index prefixes which have more equality fields
// than are necessary. The value doesn't matter, as long as its an equality
// constraint in the original query.
func calculateConstraints(q *reducedQuery) *constraints {
	ret := &constraints{
		original:        make(map[string][][]byte, len(q.eqFilters)),
		constraints:     make(map[string][][]byte, len(q.eqFilters)),
		residualMapping: make(map[string]int),
	}
	for prop, vals := range q.eqFilters {
		bvals := make([][]byte, 0, vals.Len())
		vals.Iter(func(val string) bool {
			bvals = append(bvals, []byte(val))
			return true
		})
		ret.original[prop] = bvals
		if prop == "__ancestor__" {
			// exclude __ancestor__ from the constraints.
			//
			// This is because it's handled specially during index proposal and
			// generation. Ancestor is used by ALL indexes, and so its residual value
			// in ret.original above will be sufficient.
			continue
		}
		ret.constraints[prop] = bvals
	}
	return ret
}

// getIndexes returns a set of iterator definitions. Iterating over these
// will result in matching suffixes.
func getIndexes(q *reducedQuery, s memStore) ([]*iterDefinition, error) {
	relevantIdxs := indexDefinitionSortableSlice(nil)
	if q.kind == "" {
		if coll := s.GetCollection("ents:" + q.kc.Namespace); coll != nil {
			relevantIdxs = indexDefinitionSortableSlice{{coll: coll}}
		}
	} else {
		err := error(nil)
		relevantIdxs, err = getRelevantIndexes(q, s)
		if err != nil {
			return nil, err
		}
	}
	if len(relevantIdxs) == 0 {
		return nil, ds.ErrNullQuery
	}

	// This sorts it so that relevantIdxs goes less filters -> more filters. We
	// traverse this list backwards, however, so we traverse it in more filters ->
	// less filters order.
	sort.Sort(relevantIdxs)

	constraints := calculateConstraints(q)

	ret := []*iterDefinition{}
	for !constraints.empty() || len(ret) == 0 {
		bestIdx := (*indexDefinitionSortable)(nil)
		if len(ret) == 0 {
			// if ret is empty, take the biggest relevantIdx. It's guaranteed to have
			// the greatest number of equality filters of any index in the list, and
			// we know that every equality filter will be pulled from constraints and
			// not residual.
			//
			// This also takes care of the case when the query has no equality filters,
			// in which case relevantIdxs will actually only contain one index anyway
			// :)
			bestIdx = &relevantIdxs[len(relevantIdxs)-1]
			if bestIdx.coll == nil {
				return nil, ds.ErrNullQuery
			}
		} else {
			// If ret's not empty, then we need to find the best index we can. The
			// best index will be the one with the most matching equality columns.
			// Since relevantIdxs is sorted primarially by the number of equality
			// columns, we walk down the list until the number of possible columns is
			// worse than our best-so-far.
			//
			// Traversing the list backwards goes from more filters -> less filters,
			// but also allows us to remove items from the list as we iterate over it.
			bestNumEqHits := 0
			for i := len(relevantIdxs) - 1; i >= 0; i-- {
				idx := &relevantIdxs[i]
				if len(idx.eqFilts) < bestNumEqHits {
					// if the number of filters drops below our best hit, it's never going
					// to get better than that. This index might be helpful on a later
					// loop though, so don't remove it.
					break
				}
				numHits := 0
				if idx.coll != nil {
					numHits = idx.numEqHits(constraints)
				}
				if numHits > bestNumEqHits {
					bestNumEqHits = numHits
					bestIdx = idx
				} else if numHits == 0 {
					// This index will never become useful again, so remove it.
					relevantIdxs = append(relevantIdxs[:i], relevantIdxs[i+1:]...)
				}
			}
		}
		if bestIdx == nil {
			// something is really wrong here... if relevantIdxs is !nil, then we
			// should always be able to make progress in this loop.
			impossible(fmt.Errorf("deadlock: cannot fulfil query?"))
		}
		ret = append(ret, generate(q, bestIdx, constraints))
	}

	return ret, nil
}
