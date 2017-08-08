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
	"sync"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
)

var (
	// ErrMultipleInequalityFilter is returned from Query.Finalize if you build a
	// query which has inequality filters on multiple fields.
	ErrMultipleInequalityFilter = errors.New(
		"inequality filters on multiple properties in the same Query is not allowed")

	// ErrNullQuery is returned from Query.Finalize if you build a query for which
	// there cannot possibly be any results.
	ErrNullQuery = errors.New(
		"the query is overconstrained and can never have results")
)

// Query is a builder-object for building a datastore query. It may represent
// an invalid query, but the error will only be observable when you call
// Finalize.
//
// A Query is, for the most part, not goroutine-safe. However, it is
type Query struct {
	queryFields

	// These are set by Finalize as a way to cache the 1-1 correspondence of
	// a Query to its FinalizedQuery form. err may also be set by intermediate
	// Query functions if there's a problem before finalization.
	//
	// Query implements lazy finalization, meaning that it will happen at most
	// once. This means that the finalization state and cached finalization must
	// be locked around.
	finalizeOnce sync.Once
	finalized    *FinalizedQuery
	finalizeErr  error
}

// queryFields are the Query's read-only fields.
type queryFields struct {
	kind string

	eventualConsistency bool
	keysOnly            bool
	distinct            bool

	limit  *int32
	offset *int32

	order   []IndexColumn
	project stringset.Set

	eqFilts map[string]PropertySlice

	ineqFiltProp     string
	ineqFiltLow      Property
	ineqFiltLowIncl  bool
	ineqFiltLowSet   bool
	ineqFiltHigh     Property
	ineqFiltHighIncl bool
	ineqFiltHighSet  bool

	start Cursor
	end   Cursor

	err error
}

// NewQuery returns a new Query for the given kind. If kind may be empty to
// begin a kindless query.
func NewQuery(kind string) *Query {
	return &Query{
		queryFields: queryFields{
			kind: kind,
		},
	}
}

func (q *Query) mod(cb func(*Query)) *Query {
	if q.err != nil {
		return q
	}

	ret := Query{
		queryFields: q.queryFields,
	}
	if len(q.order) > 0 {
		ret.order = make([]IndexColumn, len(q.order))
		copy(ret.order, q.order)
	}
	if q.project != nil {
		ret.project = q.project.Dup()
	}
	if len(q.eqFilts) > 0 {
		ret.eqFilts = make(map[string]PropertySlice, len(q.eqFilts))
		for k, v := range q.eqFilts {
			newV := make(PropertySlice, len(v))
			copy(newV, v)
			ret.eqFilts[k] = newV
		}
	}
	cb(&ret)
	return &ret
}

// Kind alters the kind of this query.
func (q *Query) Kind(kind string) *Query {
	return q.mod(func(q *Query) {
		q.kind = kind
	})
}

// Ancestor sets the ancestor filter for this query.
//
// If ancestor is nil, then this removes the Ancestor restriction from the
// query.
func (q *Query) Ancestor(ancestor *Key) *Query {
	return q.mod(func(q *Query) {
		if q.eqFilts == nil {
			q.eqFilts = map[string]PropertySlice{}
		}
		if ancestor == nil {
			delete(q.eqFilts, "__ancestor__")
			if len(q.eqFilts) == 0 {
				q.eqFilts = nil
			}
		} else {
			q.eqFilts["__ancestor__"] = PropertySlice{MkProperty(ancestor)}
		}
	})
}

// EventualConsistency changes the EventualConsistency setting for this query.
//
// It only has an effect on Ancestor queries and is otherwise ignored.
func (q *Query) EventualConsistency(on bool) *Query {
	return q.mod(func(q *Query) {
		q.eventualConsistency = on
	})
}

// Limit sets the limit (max items to return) for this query. If limit < 0, this
// removes the limit from the query entirely.
func (q *Query) Limit(limit int32) *Query {
	return q.mod(func(q *Query) {
		if limit < 0 {
			q.limit = nil
		} else {
			q.limit = &limit
		}
	})
}

// Offset sets the offset (number of items to skip) for this query. If
// offset < 0, this removes the offset from the query entirely.
func (q *Query) Offset(offset int32) *Query {
	return q.mod(func(q *Query) {
		if offset < 0 {
			q.offset = nil
		} else {
			q.offset = &offset
		}
	})
}

// KeysOnly makes this into a query which only returns keys (but doesn't fetch
// values). It's incompatible with projection queries.
func (q *Query) KeysOnly(on bool) *Query {
	return q.mod(func(q *Query) {
		q.keysOnly = on
	})
}

// Order sets one or more orders for this query.
func (q *Query) Order(fieldNames ...string) *Query {
	if len(fieldNames) == 0 {
		return q
	}
	return q.mod(func(q *Query) {
		for _, fn := range fieldNames {
			ic, err := ParseIndexColumn(fn)
			if err != nil {
				q.err = err
				return
			}
			if q.reserved(ic.Property) {
				return
			}
			q.order = append(q.order, ic)
		}
	})
}

// ClearOrder removes all orders from this Query.
func (q *Query) ClearOrder() *Query {
	return q.mod(func(q *Query) {
		q.order = nil
	})
}

// Project lists one or more field names to project.
func (q *Query) Project(fieldNames ...string) *Query {
	if len(fieldNames) == 0 {
		return q
	}
	return q.mod(func(q *Query) {
		for _, f := range fieldNames {
			if q.reserved(f) {
				return
			}
			if f == "__key__" {
				q.err = fmt.Errorf("cannot project on %q", f)
				return
			}
			if q.project == nil {
				q.project = stringset.New(1)
			}
			q.project.Add(f)
		}
	})
}

// Distinct makes a projection query only return distinct values. This has
// no effect on non-projection queries.
func (q *Query) Distinct(on bool) *Query {
	return q.mod(func(q *Query) {
		q.distinct = on
	})
}

// ClearProject removes all projected fields from this Query.
func (q *Query) ClearProject() *Query {
	return q.mod(func(q *Query) {
		q.project = nil
	})
}

// Start sets a starting cursor. The cursor is implementation-defined by the
// particular 'impl' you have installed.
func (q *Query) Start(c Cursor) *Query {
	return q.mod(func(q *Query) {
		q.start = c
	})
}

// End sets the ending cursor. The cursor is implementation-defined by the
// particular 'impl' you have installed.
func (q *Query) End(c Cursor) *Query {
	return q.mod(func(q *Query) {
		q.end = c
	})
}

// Eq adds one or more equality restrictions to the query.
//
// Equality filters interact with multiply-defined properties by ensuring that
// the given field has /at least one/ value which is equal to the specified
// constraint.
//
// So a query with `.Eq("thing", 1, 2)` will only return entities where the
// field "thing" is multiply defined and contains both a value of 1 and a value
// of 2.
//
// `Eq("thing", 1).Eq("thing", 2)` and `.Eq("thing", 1, 2)` have identical
// meaning.
func (q *Query) Eq(field string, values ...interface{}) *Query {
	if len(values) == 0 {
		return q
	}
	return q.mod(func(q *Query) {
		if !q.reserved(field) {
			if q.eqFilts == nil {
				q.eqFilts = make(map[string]PropertySlice, 1)
			}
			s := q.eqFilts[field]
			for _, value := range values {
				p := Property{}
				if q.err = p.SetValue(value, ShouldIndex); q.err != nil {
					return
				}
				idx := sort.Search(len(s), func(i int) bool {
					// s[i] >= p is the same as:
					return s[i].Equal(&p) || p.Less(&s[i])
				})
				if idx == len(s) || !s[idx].Equal(&p) {
					s = append(s, Property{})
					copy(s[idx+1:], s[idx:])
					s[idx] = p
				}
			}
			q.eqFilts[field] = s
		}
	})
}

func (q *Query) reserved(field string) bool {
	if field == "__key__" {
		return false
	}
	if field == "" {
		q.err = fmt.Errorf(
			"cannot filter/project on: %q", field)
		return true
	}
	if strings.HasPrefix(field, "__") && strings.HasSuffix(field, "__") {
		q.err = fmt.Errorf(
			"cannot filter/project on reserved property: %q", field)
		return true
	}
	return false
}

func (q *Query) ineqOK(field string, value Property) bool {
	if q.reserved(field) {
		return false
	}
	if field == "__key__" && value.Type() != PTKey {
		q.err = fmt.Errorf(
			"filters on %q must have type *Key (got %s)", field, value.Type())
		return false
	}
	if q.ineqFiltProp != "" && q.ineqFiltProp != field {
		q.err = ErrMultipleInequalityFilter
		return false
	}
	return true
}

// Lt imposes a 'less-than' inequality restriction on the Query.
//
// Inequality filters interact with multiply-defined properties by ensuring that
// the given field has /exactly one/ value which matches /all/ of the inequality
// constraints.
//
// So a query with `.Gt("thing", 5).Lt("thing", 10)` will only return entities
// where the field "thing" has a single value where `5 < val < 10`.
func (q *Query) Lt(field string, value interface{}) *Query {
	p := Property{}
	err := p.SetValue(value, ShouldIndex)

	if err == nil && q.ineqFiltHighSet {
		if q.ineqFiltHigh.Less(&p) {
			return q
		} else if q.ineqFiltHigh.Equal(&p) && !q.ineqFiltHighIncl {
			return q
		}
	}

	return q.mod(func(q *Query) {
		if q.err = err; err != nil {
			return
		}
		if q.ineqOK(field, p) {
			q.ineqFiltProp = field
			q.ineqFiltHighSet = true
			q.ineqFiltHigh = p
			q.ineqFiltHighIncl = false
		}
	})
}

// Lte imposes a 'less-than-or-equal' inequality restriction on the Query.
//
// Inequality filters interact with multiply-defined properties by ensuring that
// the given field has /exactly one/ value which matches /all/ of the inequality
// constraints.
//
// So a query with `.Gt("thing", 5).Lt("thing", 10)` will only return entities
// where the field "thing" has a single value where `5 < val < 10`.
func (q *Query) Lte(field string, value interface{}) *Query {
	p := Property{}
	err := p.SetValue(value, ShouldIndex)

	if err == nil && q.ineqFiltHighSet {
		if q.ineqFiltHigh.Less(&p) {
			return q
		} else if q.ineqFiltHigh.Equal(&p) {
			return q
		}
	}

	return q.mod(func(q *Query) {
		if q.err = err; err != nil {
			return
		}
		if q.ineqOK(field, p) {
			q.ineqFiltProp = field
			q.ineqFiltHighSet = true
			q.ineqFiltHigh = p
			q.ineqFiltHighIncl = true
		}
	})
}

// Gt imposes a 'greater-than' inequality restriction on the Query.
//
// Inequality filters interact with multiply-defined properties by ensuring that
// the given field has /exactly one/ value which matches /all/ of the inequality
// constraints.
//
// So a query with `.Gt("thing", 5).Lt("thing", 10)` will only return entities
// where the field "thing" has a single value where `5 < val < 10`.
func (q *Query) Gt(field string, value interface{}) *Query {
	p := Property{}
	err := p.SetValue(value, ShouldIndex)

	if err == nil && q.ineqFiltLowSet {
		if p.Less(&q.ineqFiltLow) {
			return q
		} else if p.Equal(&q.ineqFiltLow) && !q.ineqFiltLowIncl {
			return q
		}
	}

	return q.mod(func(q *Query) {
		if q.err = err; err != nil {
			return
		}
		if q.ineqOK(field, p) {
			q.ineqFiltProp = field
			q.ineqFiltLowSet = true
			q.ineqFiltLow = p
			q.ineqFiltLowIncl = false
		}
	})
}

// Gte imposes a 'greater-than-or-equal' inequality restriction on the Query.
//
// Inequality filters interact with multiply-defined properties by ensuring that
// the given field has /exactly one/ value which matches /all/ of the inequality
// constraints.
//
// So a query with `.Gt("thing", 5).Lt("thing", 10)` will only return entities
// where the field "thing" has a single value where `5 < val < 10`.
func (q *Query) Gte(field string, value interface{}) *Query {
	p := Property{}
	err := p.SetValue(value, ShouldIndex)

	if err == nil && q.ineqFiltLowSet {
		if p.Less(&q.ineqFiltLow) {
			return q
		} else if p.Equal(&q.ineqFiltLow) {
			return q
		}
	}

	return q.mod(func(q *Query) {
		if q.err = err; err != nil {
			return
		}
		if q.ineqOK(field, p) {
			q.ineqFiltProp = field
			q.ineqFiltLowSet = true
			q.ineqFiltLow = p
			q.ineqFiltLowIncl = true
		}
	})
}

// ClearFilters clears all equality and inequality filters from the Query. It
// does not clear the Ancestor filter if one is defined.
func (q *Query) ClearFilters() *Query {
	return q.mod(func(q *Query) {
		anc := q.eqFilts["__ancestor__"]
		if anc != nil {
			q.eqFilts = map[string]PropertySlice{"__ancestor__": anc}
		} else {
			q.eqFilts = nil
		}
		q.ineqFiltLowSet = false
		q.ineqFiltHighSet = false
	})
}

// Finalize converts this Query to a FinalizedQuery. If the Query has any
// inconsistencies or violates any of the query rules, that will be returned
// here.
func (q *Query) Finalize() (*FinalizedQuery, error) {
	if q.err != nil {
		return nil, q.err
	}

	q.finalizeOnce.Do(func() {
		q.finalized, q.finalizeErr = q.finalizeImpl()
	})
	return q.finalized, q.finalizeErr
}

func (q *Query) finalizeImpl() (*FinalizedQuery, error) {
	ancestor := (*Key)(nil)
	if slice, ok := q.eqFilts["__ancestor__"]; ok {
		ancestor = slice[0].Value().(*Key)
	}

	err := func() error {

		if q.kind == "" { // kindless query checks
			if q.ineqFiltProp != "" && q.ineqFiltProp != "__key__" {
				return fmt.Errorf(
					"kindless queries can only filter on __key__, got %q", q.ineqFiltProp)
			}
			allowedEqs := 0
			if ancestor != nil {
				allowedEqs = 1
			}
			if len(q.eqFilts) > allowedEqs {
				return fmt.Errorf("kindless queries may not have any equality filters")
			}
			for _, o := range q.order {
				if o.Property != "__key__" || o.Descending {
					return fmt.Errorf("invalid order for kindless query: %#v", o)
				}
			}
		}

		if q.keysOnly && q.project != nil && q.project.Len() > 0 {
			return errors.New("cannot project a keysOnly query")
		}

		if q.ineqFiltProp != "" {
			if len(q.order) > 0 && q.order[0].Property != q.ineqFiltProp {
				return fmt.Errorf(
					"first sort order must match inequality filter: %q v %q",
					q.order[0].Property, q.ineqFiltProp)
			}
			if q.ineqFiltLowSet && q.ineqFiltHighSet {
				if q.ineqFiltHigh.Less(&q.ineqFiltLow) ||
					(q.ineqFiltHigh.Equal(&q.ineqFiltLow) &&
						(!q.ineqFiltLowIncl || !q.ineqFiltHighIncl)) {
					return ErrNullQuery
				}
			}
			if q.ineqFiltProp == "__key__" {
				if q.ineqFiltLowSet {
					if ancestor != nil && !q.ineqFiltLow.Value().(*Key).HasAncestor(ancestor) {
						return fmt.Errorf(
							"inequality filters on __key__ must be descendants of the __ancestor__")
					}
				}
				if q.ineqFiltHighSet {
					if ancestor != nil && !q.ineqFiltHigh.Value().(*Key).HasAncestor(ancestor) {
						return fmt.Errorf(
							"inequality filters on __key__ must be descendants of the __ancestor__")
					}
				}
			}
		}

		err := error(nil)
		if q.project != nil {
			q.project.Iter(func(p string) bool {
				if _, iseq := q.eqFilts[p]; iseq {
					err = fmt.Errorf("cannot project on equality filter field: %s", p)
					return false
				}
				return true
			})
		}
		return err
	}()
	if err != nil {
		return nil, err
	}

	ret := &FinalizedQuery{
		original: q,
		kind:     q.kind,

		keysOnly:             q.keysOnly,
		eventuallyConsistent: q.eventualConsistency || ancestor == nil,
		limit:                q.limit,
		offset:               q.offset,
		start:                q.start,
		end:                  q.end,

		eqFilts: q.eqFilts,

		ineqFiltProp:     q.ineqFiltProp,
		ineqFiltLow:      q.ineqFiltLow,
		ineqFiltLowIncl:  q.ineqFiltLowIncl,
		ineqFiltLowSet:   q.ineqFiltLowSet,
		ineqFiltHigh:     q.ineqFiltHigh,
		ineqFiltHighIncl: q.ineqFiltHighIncl,
		ineqFiltHighSet:  q.ineqFiltHighSet,
	}
	// If a starting cursor is provided, ignore the offset, as it would have been
	// accounted for in the query that produced the cursor.
	if ret.start != nil {
		ret.offset = nil
	}

	if q.project != nil {
		ret.project = q.project.ToSlice()
		ret.distinct = q.distinct && q.project.Len() > 0

		// If we're DISTINCT && have an inequality filter, we must project that
		// inequality property as well.
		if ret.distinct && ret.ineqFiltProp != "" && !q.project.Has(ret.ineqFiltProp) {
			ret.project = append([]string{ret.ineqFiltProp}, ret.project...)
		}
	}

	seenOrders := stringset.New(len(q.order))

	// if len(q.order) > 0, we already enforce that the first order
	// is the same as the inequality above. Otherwise we need to add it.
	if len(q.order) == 0 && q.ineqFiltProp != "" {
		ret.orders = []IndexColumn{{Property: q.ineqFiltProp}}
		seenOrders.Add(q.ineqFiltProp)
	}

	// drop orders where there's an equality filter
	//   https://cloud.google.com/appengine/docs/go/datastore/queries#sort_orders_are_ignored_on_properties_with_equality_filters
	// Deduplicate orders
	for _, o := range q.order {
		if _, iseq := q.eqFilts[o.Property]; !iseq {
			if seenOrders.Add(o.Property) {
				ret.orders = append(ret.orders, o)
			}
		}
	}

	// Add any projection columns not mentioned in the user-defined order as
	// ASCENDING orders. Technically we could be smart and automatically use
	// a DESCENDING ordered index, if it fit, but the logic gets insane, since all
	// suffixes of all used indexes need to be PRECISELY equal (and so you'd have
	// to hunt/invalidate/something to find the combination of indexes that are
	// compatible with each other as well as the query). If you want to use
	// a DESCENDING column, just add it to the user sort order, and this loop will
	// not synthesize a new suffix entry for it.
	//
	// NOTE: if you want to use an index that sorts by -__key__, you MUST
	// include all of the projected fields for that index in the order explicitly.
	// Otherwise the generated orders will be wacky. So:
	//   Query("Foo").Project("A", "B").Order("A").Order("-__key__")
	//
	// will turn into a orders of:
	//   A, ASCENDING
	//   __key__, DESCENDING
	//   B, ASCENDING
	//   __key__, ASCENDING
	//
	// To prevent this, your query should have another Order("B") clause before
	// the -__key__ clause.
	if len(ret.project) > 0 {
		sort.Strings(ret.project)
		for _, p := range ret.project {
			if !seenOrders.Has(p) {
				ret.orders = append(ret.orders, IndexColumn{Property: p})
			}
		}
	}

	// If the suffix format ends with __key__ already (e.g. .Order("__key__")),
	// then we're good to go. Otherwise we need to add it as the last bit of the
	// suffix, since all indexes implicitly have it as the last column.
	if len(ret.orders) == 0 || ret.orders[len(ret.orders)-1].Property != "__key__" {
		ret.orders = append(ret.orders, IndexColumn{Property: "__key__"})
	}

	return ret, nil
}

func (q *Query) String() string {
	ret := &bytes.Buffer{}
	needComma := false
	p := func(fmtStr string, stuff ...interface{}) {
		if needComma {
			if _, err := ret.WriteString(", "); err != nil {
				panic(err)
			}
		}
		needComma = true
		fmt.Fprintf(ret, fmtStr, stuff...)
	}
	if _, err := ret.WriteString("Query("); err != nil {
		panic(err)
	}
	if q.err != nil {
		p("ERROR=%q", q.err.Error())
	}

	// Filters
	if q.kind != "" {
		p("Kind=%q", q.kind)
	}
	if q.eqFilts["__ancestor__"] != nil {
		p("Ancestor=%s", q.eqFilts["__ancestor__"][0].Value().(*Key).String())
	}
	for prop, vals := range q.eqFilts {
		if prop == "__ancestor__" {
			continue
		}
		for _, v := range vals {
			p("Filter(%q == %s)", prop, v.GQL())
		}
	}
	if q.ineqFiltProp != "" {
		if q.ineqFiltLowSet {
			op := ">"
			if q.ineqFiltLowIncl {
				op = ">="
			}
			p("Filter(%q %s %s)", q.ineqFiltProp, op, q.ineqFiltLow.GQL())
		}
		if q.ineqFiltHighSet {
			op := "<"
			if q.ineqFiltHighIncl {
				op = "<="
			}
			p("Filter(%q %s %s)", q.ineqFiltProp, op, q.ineqFiltHigh.GQL())
		}
	}

	// Order
	if len(q.order) > 0 {
		orders := make([]string, len(q.order))
		for i, o := range q.order {
			orders[i] = o.String()
		}
		p("Order(%s)", strings.Join(orders, ", "))
	}

	// Projection
	if q.project != nil && q.project.Len() > 0 {
		f := "Project(%s)"
		if q.distinct {
			f = "Project[DISTINCT](%s)"
		}
		p(f, strings.Join(q.project.ToSlice(), ", "))
	}

	// Cursors
	if q.start != nil {
		p("Start(%q)", q.start.String())
	}
	if q.end != nil {
		p("End(%q)", q.end.String())
	}

	// Modifiiers
	if q.limit != nil {
		p("Limit=%d", *q.limit)
	}
	if q.offset != nil {
		p("Offset=%d", *q.offset)
	}
	if q.eventualConsistency {
		p("EventualConsistency")
	}
	if q.keysOnly {
		p("KeysOnly")
	}

	if _, err := ret.WriteRune(')'); err != nil {
		panic(err)
	}

	return ret.String()
}
