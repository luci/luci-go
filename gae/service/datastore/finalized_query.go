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

package datastore

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

// FinalizedQuery is the representation of a Query which has been normalized.
//
// It contains only fully-specified, non-redundant, non-conflicting information
// pertaining to the Query to run. It can only represent a valid query.
type FinalizedQuery struct {
	original *Query

	kind                 string
	eventuallyConsistent bool
	distinct             bool
	keysOnly             bool

	limit  *int32
	offset *int32

	start Cursor
	end   Cursor

	project []string
	orders  []IndexColumn

	eqFilts map[string]PropertySlice

	ineqFiltProp     string
	ineqFiltLow      Property
	ineqFiltLowIncl  bool
	ineqFiltLowSet   bool
	ineqFiltHigh     Property
	ineqFiltHighIncl bool
	ineqFiltHighSet  bool
}

// Original returns the original Query object from which this FinalizedQuery was
// derived.
func (q *FinalizedQuery) Original() *Query {
	return q.original
}

// Kind returns the datastore 'Kind' over which this query operates. It may be
// empty for a kindless query.
func (q *FinalizedQuery) Kind() string {
	return q.kind
}

// EventuallyConsistent returns true iff this query will be eventually
// consistent. This is true when the query is a non-ancestor query, or when it's
// an ancestory query with the 'EventualConsistency(true)' option set.
func (q *FinalizedQuery) EventuallyConsistent() bool {
	return q.eventuallyConsistent
}

// Project is the list of fields that this query projects on, or empty if this
// is not a projection query.
func (q *FinalizedQuery) Project() []string {
	if len(q.project) == 0 {
		return nil
	}
	ret := make([]string, len(q.project))
	copy(ret, q.project)
	return ret
}

// Distinct returnst true iff this is a distinct projection query. It will never
// be true for non-projection queries.
func (q *FinalizedQuery) Distinct() bool {
	return q.distinct
}

// KeysOnly returns true iff this query will only return keys (as opposed to a
// normal or projection query).
func (q *FinalizedQuery) KeysOnly() bool {
	return q.keysOnly
}

// Limit returns the maximum number of responses this query will retrieve, and a
// boolean indicating if the limit is set.
func (q *FinalizedQuery) Limit() (int32, bool) {
	if q.limit != nil {
		return *q.limit, true
	}
	return 0, false
}

// Offset returns the number of responses this query will skip before returning
// data, and a boolean indicating if the offset is set.
func (q *FinalizedQuery) Offset() (int32, bool) {
	if q.offset != nil {
		return *q.offset, true
	}
	return 0, false
}

// Orders returns the sort orders that this query will use, including all orders
// implied by the projections, and the implicit __key__ order at the end.
func (q *FinalizedQuery) Orders() []IndexColumn {
	ret := make([]IndexColumn, len(q.orders))
	copy(ret, q.orders)
	return ret
}

// Bounds returns the start and end Cursors. One or both may be nil. The Cursors
// returned are implementation-specific depending on the actual RawInterface
// implementation and the filters installed (if the filters interfere with
// Cursor production).
func (q *FinalizedQuery) Bounds() (start, end Cursor) {
	return q.start, q.end
}

// Ancestor returns the ancestor filter key, if any. This is a convenience
// function for getting the value from EqFilters()["__ancestor__"].
func (q *FinalizedQuery) Ancestor() *Key {
	if anc, ok := q.eqFilts["__ancestor__"]; ok {
		return anc[0].Value().(*Key)
	}
	return nil
}

// EqFilters returns all the equality filters. The map key is the field name
// and the PropertySlice is the values that field should equal.
//
// This includes a special equality filter on "__ancestor__". If "__ancestor__"
// is present in the result, it's guaranteed to have 1 value in the
// PropertySlice which is of type *Key.
func (q *FinalizedQuery) EqFilters() map[string]PropertySlice {
	ret := make(map[string]PropertySlice, len(q.eqFilts))
	for k, v := range q.eqFilts {
		newV := make(PropertySlice, len(v))
		copy(newV, v)
		ret[k] = newV
	}
	return ret
}

// IneqFilterProp returns the inequality filter property name, if one is used
// for this filter. An empty return value means that this query does not
// contain any inequality filters.
func (q *FinalizedQuery) IneqFilterProp() string {
	return q.ineqFiltProp
}

// IneqFilterLow returns the field name, operator and value for the low-side
// inequality filter. If the returned field name is "", it means that there's
// no lower inequality bound on this query.
//
// If field is non-empty, op may have the values ">" or ">=".
func (q *FinalizedQuery) IneqFilterLow() (field, op string, val Property) {
	if q.ineqFiltLowSet {
		field = q.ineqFiltProp
		val = q.ineqFiltLow
		op = ">"
		if q.ineqFiltLowIncl {
			op = ">="
		}
	}
	return
}

// IneqFilterHigh returns the field name, operator and value for the high-side
// inequality filter. If the returned field name is "", it means that there's
// no upper inequality bound on this query.
//
// If field is non-empty, op may have the values "<" or "<=".
func (q *FinalizedQuery) IneqFilterHigh() (field, op string, val Property) {
	if q.ineqFiltHighSet {
		field = q.ineqFiltProp
		val = q.ineqFiltHigh
		op = "<"
		if q.ineqFiltHighIncl {
			op = "<="
		}
	}
	return
}

var escaper = strings.NewReplacer(
	"\\%", `\%`,
	"\\_", `\_`,
	"\\", `\\`,
	"\x00", `\0`,
	"\b", `\b`,
	"\n", `\n`,
	"\r", `\r`,
	"\t", `\t`,
	"\x1A", `\Z`,
	"'", `\'`,
	"\"", `\"`,
	"`", "\\`",
)

func gqlQuoteName(s string) string {
	return fmt.Sprintf("`%s`", escaper.Replace(s))
}

func gqlQuoteString(s string) string {
	return fmt.Sprintf(`"%s"`, escaper.Replace(s))
}

// GQL returns a correctly formatted Cloud Datastore GQL expression which
// is equivalent to this query.
//
// The flavor of GQL that this emits is defined here:
//   https://cloud.google.com/datastore/docs/apis/gql/gql_reference
//
// NOTE: Cursors are omitted because currently there's currently no syntax for
// literal cursors.
//
// NOTE: GeoPoint values are emitted with speculated future syntax. There is
// currently no syntax for literal GeoPoint values.
func (q *FinalizedQuery) GQL() string {
	ret := bytes.Buffer{}

	ws := func(s string) {
		_, err := ret.WriteString(s)
		if err != nil {
			panic(err)
		}
	}

	ws("SELECT")
	if len(q.project) != 0 {
		if q.distinct {
			ws(" DISTINCT")
		}
		proj := make([]string, len(q.project))
		for i, p := range q.project {
			proj[i] = gqlQuoteName(p)
		}
		ws(" ")
		ws(strings.Join(proj, ", "))
	} else if q.keysOnly {
		ws(" __key__")
	} else {
		ws(" *")
	}

	if q.kind != "" {
		fmt.Fprintf(&ret, " FROM %s", gqlQuoteName(q.kind))
	}

	filts := []string(nil)
	anc := Property{}
	if len(q.eqFilts) > 0 {
		eqProps := make([]string, 0, len(q.eqFilts))
		for k, v := range q.eqFilts {
			if k == "__ancestor__" {
				anc = v[0]
				continue
			}
			eqProps = append(eqProps, k)
		}
		sort.Strings(eqProps)
		for _, k := range eqProps {
			vals := q.eqFilts[k]
			k = gqlQuoteName(k)
			for _, v := range vals {
				if v.Type() == PTNull {
					filts = append(filts, fmt.Sprintf("%s IS NULL", k))
				} else {
					filts = append(filts, fmt.Sprintf("%s = %s", k, v.GQL()))
				}
			}
		}
	}
	if q.ineqFiltProp != "" {
		for _, f := range [](func() (p, op string, v Property)){q.IneqFilterLow, q.IneqFilterHigh} {
			prop, op, v := f()
			if prop != "" {
				filts = append(filts, fmt.Sprintf("%s %s %s", gqlQuoteName(prop), op, v.GQL()))
			}
		}
	}
	if anc.propType != PTNull {
		filts = append(filts, fmt.Sprintf("__key__ HAS ANCESTOR %s", anc.GQL()))
	}
	if len(filts) > 0 {
		fmt.Fprintf(&ret, " WHERE %s", strings.Join(filts, " AND "))
	}

	if len(q.orders) > 0 {
		orders := make([]string, len(q.orders))
		for i, col := range q.orders {
			orders[i] = col.GQL()
		}
		fmt.Fprintf(&ret, " ORDER BY %s", strings.Join(orders, ", "))
	}

	if q.limit != nil {
		fmt.Fprintf(&ret, " LIMIT %d", *q.limit)
	}
	if q.offset != nil {
		fmt.Fprintf(&ret, " OFFSET %d", *q.offset)
	}

	return ret.String()
}

func (q *FinalizedQuery) String() string {
	// TODO(riannucci): make a more compact go-like representation here.
	return q.GQL()
}

// Valid returns true iff this FinalizedQuery is valid in the provided
// KeyContext's App ID and Namespace.
//
// This checks the ancestor filter (if any), as well as the inequality filters
// if they filter on '__key__'.
//
// In particular, it does NOT validate equality filters which happen to have
// values of type PTKey, nor does it validate inequality filters that happen to
// have values of type PTKey (but don't filter on the magic '__key__' field).
func (q *FinalizedQuery) Valid(kc KeyContext) error {
	anc := q.Ancestor()
	if anc != nil {
		switch {
		case !anc.Valid(false, kc):
			return MakeErrInvalidKey("ancestor [%s] is not valid in context %s", anc, kc).Err()
		case anc.IsIncomplete():
			return MakeErrInvalidKey("ancestor [%s] is incomplete", anc).Err()
		}
	}

	if q.ineqFiltProp == "__key__" {
		if q.ineqFiltLowSet {
			if k := q.ineqFiltLow.Value().(*Key); !k.Valid(false, kc) {
				return MakeErrInvalidKey(
					"low inequality filter key [%s] is not valid in context %s", k, kc).Err()
			}
		}
		if q.ineqFiltHighSet {
			if k := q.ineqFiltHigh.Value().(*Key); !k.Valid(false, kc) {
				return MakeErrInvalidKey(
					"high inequality filter key [%s] is not valid in context %s", k, kc).Err()
			}
		}
	}
	return nil
}
