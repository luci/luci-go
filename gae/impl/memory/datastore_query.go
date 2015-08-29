// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"strings"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/datastore/serialize"
	"github.com/luci/luci-go/common/cmpbin"
)

// MaxQueryComponents was lifted from a hard-coded constant in dev_appserver.
// No idea if it's a real limit or just a convenience in the current dev
// appserver implementation.
const MaxQueryComponents = 100

var errQueryDone = errors.New("query is done")

type queryOp int

const (
	qInvalid queryOp = iota
	qEqual
	qLessThan
	qLessEq
	qGreaterEq
	qGreaterThan
)

var queryOpMap = map[string]queryOp{
	"=":  qEqual,
	"<":  qLessThan,
	"<=": qLessEq,
	">=": qGreaterEq,
	">":  qGreaterThan,
}

type queryFilter struct {
	prop  string
	op    queryOp
	value interface{}
}

func parseFilter(f string) (prop string, op queryOp, err error) {
	toks := strings.SplitN(strings.TrimSpace(f), " ", 2)
	if len(toks) != 2 {
		err = errors.New("datastore: invalid filter: " + f)
	} else {
		op = queryOpMap[toks[1]]
		if op == qInvalid {
			err = fmt.Errorf("datastore: invalid operator %q in filter %q", toks[1], f)
		} else {
			prop = toks[0]
		}
	}
	return
}

// A queryCursor is:
//   {#orders} ++ IndexColumn* ++ RawRowData
//   IndexColumn will always contain __key__ as the last column, and so #orders
//     must always be >= 1
type queryCursor []byte

func newCursor(s string) (ds.Cursor, error) {
	d, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("Failed to Base64-decode cursor: %s", err)
	}
	c := queryCursor(d)
	if _, _, err := c.decode(); err != nil {
		return nil, err
	}
	return c, nil
}

func (q queryCursor) String() string { return base64.URLEncoding.EncodeToString([]byte(q)) }

// decode returns the encoded IndexColumns, the raw row (cursor) data, or an
// error.
func (q queryCursor) decode() ([]ds.IndexColumn, []byte, error) {
	buf := bytes.NewBuffer([]byte(q))
	count, _, err := cmpbin.ReadUint(buf)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid cursor: bad prefix number")
	}

	if count == 0 || count > ds.MaxIndexColumns {
		return nil, nil, fmt.Errorf("invalid cursor: bad column count %d", count)
	}

	if count == 0 {
		return nil, nil, fmt.Errorf("invalid cursor: zero prefix number")
	}

	cols := make([]ds.IndexColumn, count)
	for i := range cols {
		if cols[i], err = serialize.ReadIndexColumn(buf); err != nil {
			return nil, nil, fmt.Errorf("invalid cursor: unable to decode IndexColumn %d: %s", i, err)
		}
	}

	if cols[len(cols)-1].Property != "__key__" {
		return nil, nil, fmt.Errorf("invalid cursor: last column was not __key__: %v", cols[len(cols)-1])
	}

	return cols, buf.Bytes(), nil
}

type queryIneqFilter struct {
	prop string

	start []byte
	end   []byte
}

// constrain 'folds' a new inequality into the current inequality filter.
//
// It will bump the end bound down, or the start bound up, assuming the incoming
// constraint does so.
//
// It returns true iff the filter is overconstrained (i.e.  start > end)
func (q *queryIneqFilter) constrain(op queryOp, val []byte) bool {
	switch op {
	case qLessEq:
		val = increment(val)
		fallthrough
	case qLessThan:
		// adjust upper bound downwards
		if q.end == nil || bytes.Compare(q.end, val) > 0 {
			q.end = val
		}

	case qGreaterThan:
		val = increment(val)
		fallthrough
	case qGreaterEq:
		// adjust lower bound upwards
		if q.start == nil || bytes.Compare(q.start, val) < 0 {
			q.start = val
		}

	default:
		impossible(fmt.Errorf("constrain cannot handle filter op %d", op))
	}

	if q.start != nil && q.end != nil {
		return bytes.Compare(q.start, q.end) >= 0
	}
	return false
}

type queryImpl struct {
	ns string

	kind string

	// prop -> encoded values (which are ds.Property objects)
	// "__ancestor__" is the key for Ancestor queries.
	eqFilters          map[string]stringSet
	ineqFilter         queryIneqFilter
	order              []ds.IndexColumn
	startCursor        []byte
	startCursorColumns []ds.IndexColumn
	endCursor          []byte
	endCursorColumns   []ds.IndexColumn

	// All of these are applied in post (e.g. not during the native index scan).
	distinct            bool
	eventualConsistency bool
	keysOnly            bool
	limitSet            bool
	limit               int32
	offset              int32
	project             []string

	err error
}

var _ ds.Query = (*queryImpl)(nil)

func sortOrdersEqual(as, bs []ds.IndexColumn) bool {
	if len(as) != len(bs) {
		return false
	}
	for i, a := range as {
		if a != bs[i] {
			return false
		}
	}
	return true
}

func (q *queryImpl) reduce(ns string, isTxn bool) (*reducedQuery, error) {
	if q.err != nil {
		return nil, q.err
	}
	if ns != q.ns {
		return nil, errors.New(
			"gae/memory: Namespace mismatched. Query and Datastore don't agree " +
				"on the current namespace")
	}
	if isTxn && q.eqFilters["__ancestor__"] == nil {
		return nil, errors.New(
			"gae/memory: Only ancestor queries are allowed inside transactions")
	}
	if q.numComponents() > MaxQueryComponents {
		return nil, fmt.Errorf(
			"gae/memory: query is too large. may not have more than "+
				"%d filters + sort orders + ancestor total: had %d",
			MaxQueryComponents, q.numComponents())
	}
	if len(q.project) == 0 && q.distinct {
		// This must be delayed, because q.Distinct().Project("foo") is a valid
		// construction. If we checked this in Distinct, it could be too early, and
		// checking it in Project doesn't matter.
		return nil, errors.New(
			"gae/memory: Distinct() only makes sense on projection queries.")
	}
	if q.eqFilters["__ancestor__"] != nil && q.ineqFilter.prop == "__key__" {
		anc := []byte(nil)
		for k := range q.eqFilters["__ancestor__"] {
			anc = []byte(k)
			break
		}
		anc = anc[:len(anc)-1]
		if q.ineqFilter.start != nil && !bytes.HasPrefix(q.ineqFilter.start, anc) {
			return nil, errors.New(
				"gae/memory: __key__ inequality filter has a value outside of Ancestor()")
		}
		if q.ineqFilter.end != nil && !bytes.HasPrefix(q.ineqFilter.end, anc) {
			return nil, errors.New(
				"gae/memory: __key__ inequality filter has a value outside of Ancestor()")
		}
	}

	ret := &reducedQuery{
		ns:           q.ns,
		kind:         q.kind,
		eqFilters:    q.eqFilters,
		suffixFormat: q.order,
	}

	// if len(q.suffixFormat) > 0, queryImpl already enforces that the first order
	// is the same as the inequality. Otherwise we need to add it.
	if len(ret.suffixFormat) == 0 && q.ineqFilter.prop != "" {
		ret.suffixFormat = []ds.IndexColumn{{Property: q.ineqFilter.prop}}
	}

	// The inequality is specified in natural (ascending) order in the query's
	// Filter syntax, but the order information may indicate to use a descending
	// index column for it. If that's the case, then we must invert, swap and
	// increment the inequality endpoints.
	//
	// Invert so that the desired numbers are represented correctly in the index.
	// Swap so that our iterators still go from >= start to < end.
	// Increment so that >= and < get correctly bounded (since the iterator is
	//   still using natrual bytes ordering)
	if q.ineqFilter.prop != "" && ret.suffixFormat[0].Direction == ds.DESCENDING {
		hi, lo := []byte(nil), []byte(nil)
		if len(q.ineqFilter.end) > 0 {
			hi = increment(invert(q.ineqFilter.end))
		}
		if len(q.ineqFilter.start) > 0 {
			lo = increment(invert(q.ineqFilter.start))
		}
		q.ineqFilter.end, q.ineqFilter.start = lo, hi
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
	// Otherwise the generated suffixFormat will be wacky. So:
	//   Query("Foo").Project("A", "B").Order("A").Order("-__key__")
	//
	// will turn into a suffixFormat of:
	//   A, ASCENDING
	//   __key__, DESCENDING
	//   B, ASCENDING
	//   __key__, ASCENDING
	//
	// To prevent this, your query should have another Order("B") clause before
	// the -__key__ clause.
	originalStop := len(ret.suffixFormat)
	for _, p := range q.project {
		needAdd := true
		// originalStop prevents this loop from getting longer every time we add
		// a projected property.
		for _, col := range ret.suffixFormat[:originalStop] {
			if col.Property == p {
				needAdd = false
				break
			}
		}
		if needAdd {
			ret.suffixFormat = append(ret.suffixFormat, ds.IndexColumn{Property: p})
		}
	}

	// If the suffix format ends with __key__ already (e.g. .Order("__key__")),
	// then we're good to go. Otherwise we need to add it as the last bit of the
	// suffix, since all indexes implicitly have it as the last column.
	if len(ret.suffixFormat) == 0 || ret.suffixFormat[len(ret.suffixFormat)-1].Property != "__key__" {
		ret.suffixFormat = append(ret.suffixFormat, ds.IndexColumn{Property: "__key__"})
	}

	// Now we check the start and end cursors.
	//
	// Cursors are composed of a list of IndexColumns at the beginning, followed
	// by the raw bytes to use for the suffix. The cursor is only valid if all of
	// its IndexColumns match our proposed suffixFormat, as calculated above.
	ret.start = q.ineqFilter.start
	if q.startCursor != nil {
		if !sortOrdersEqual(q.startCursorColumns, ret.suffixFormat) {
			return nil, errors.New("gae/memory: start cursor is invalid for this query.")
		}
		if ret.start == nil || bytes.Compare(ret.start, q.startCursor) < 0 {
			ret.start = q.startCursor
		}
	}

	ret.end = q.ineqFilter.end
	if q.endCursor != nil {
		if !sortOrdersEqual(q.endCursorColumns, ret.suffixFormat) {
			return nil, errors.New("gae/memory: end cursor is invalid for this query.")
		}
		if ret.end == nil || bytes.Compare(q.endCursor, ret.end) < 0 {
			ret.end = q.endCursor
		}
	}

	// Finally, verify that we could even /potentially/ do work. If we have
	// overlapping range ends, then we don't have anything to do.
	if ret.end != nil && bytes.Compare(ret.start, ret.end) >= 0 {
		return nil, errQueryDone
	}

	ret.numCols = len(ret.suffixFormat)
	for prop, vals := range ret.eqFilters {
		if len(ret.suffixFormat) == 1 && prop == "__ancestor__" {
			continue
		}
		ret.numCols += len(vals)
	}

	return ret, nil
}

func (q *queryImpl) numComponents() int {
	numComponents := len(q.order)
	if q.ineqFilter.prop != "" {
		if q.ineqFilter.start != nil {
			numComponents++
		}
		if q.ineqFilter.end != nil {
			numComponents++
		}
	}
	for _, v := range q.eqFilters {
		numComponents += len(v)
	}
	return numComponents
}

// checkMutateClone sees if the query has an error. If not, it clones the query,
// and assigns the output of `check` to the query error slot. If check returns
// nil, it calls `mutate` on the cloned query. The (possibly new) query is then
// returned.
func (q *queryImpl) checkMutateClone(check func() error, mutate func(*queryImpl)) *queryImpl {
	if q.err != nil {
		return q
	}
	nq := *q
	nq.eqFilters = make(map[string]stringSet, len(q.eqFilters))
	for prop, vals := range q.eqFilters {
		nq.eqFilters[prop] = vals.dup()
	}
	nq.order = make([]ds.IndexColumn, len(q.order))
	copy(nq.order, q.order)
	nq.project = make([]string, len(q.project))
	copy(nq.project, q.project)
	if check != nil {
		nq.err = check()
	}
	if nq.err == nil {
		mutate(&nq)
	}
	return &nq
}

func (q *queryImpl) Ancestor(k ds.Key) ds.Query {
	return q.checkMutateClone(
		func() error {
			if k == nil {
				// SDK has an explicit nil-check
				return errors.New("datastore: nil query ancestor")
			}
			if k.Namespace() != q.ns {
				return fmt.Errorf("bad namespace: %q (expected %q)", k.Namespace(), q.ns)
			}
			if !k.Valid(false, globalAppID, q.ns) {
				// technically the SDK implementation does a Weird Thing (tm) if both the
				// stringID and intID are set on a key; it only serializes the stringID in
				// the proto. This means that if you set the Ancestor to an invalid key,
				// you'll never actually hear about it. Instead of doing that insanity, we
				// just swap to an error here.
				return ds.ErrInvalidKey
			}
			if q.eqFilters["__ancestor__"] != nil {
				return errors.New("cannot have more than one ancestor")
			}
			return nil
		},
		func(q *queryImpl) {
			q.addEqFilt("__ancestor__", ds.MkProperty(k))
		})
}

func (q *queryImpl) Distinct() ds.Query {
	return q.checkMutateClone(nil, func(q *queryImpl) {
		q.distinct = true
	})
}

func (q *queryImpl) addEqFilt(prop string, p ds.Property) {
	binVal := string(serialize.ToBytes(p))
	if cur, ok := q.eqFilters[prop]; !ok {
		q.eqFilters[prop] = stringSet{binVal: {}}
	} else {
		cur.add(binVal)
	}
}

func (q *queryImpl) Filter(fStr string, val interface{}) ds.Query {
	prop := ""
	op := qInvalid
	p := ds.Property{}
	return q.checkMutateClone(
		func() error {
			var err error
			prop, op, err = parseFilter(fStr)
			if err != nil {
				return err
			}

			if q.kind == "" && prop != "__key__" {
				// https://cloud.google.com/appengine/docs/go/datastore/queries#Go_Kindless_queries
				return fmt.Errorf(
					"kindless queries can only filter on __key__, got %q", fStr)
			}

			err = p.SetValue(val, ds.ShouldIndex)
			if err != nil {
				return err
			}

			if p.Type() == ds.PTKey {
				if !p.Value().(ds.Key).Valid(false, globalAppID, q.ns) {
					return ds.ErrInvalidKey
				}
			}

			if prop == "__key__" {
				if op == qEqual {
					return fmt.Errorf(
						"query equality filter on __key__ is silly: %q", fStr)
				}
				if p.Type() != ds.PTKey {
					return fmt.Errorf("__key__ filter value is not a key: %T", val)
				}
			} else if strings.HasPrefix(prop, "__") && strings.HasSuffix(prop, "__") {
				return fmt.Errorf("filter on reserved property: %q", prop)
			}

			if op != qEqual {
				if q.ineqFilter.prop != "" && q.ineqFilter.prop != prop {
					return fmt.Errorf(
						"inequality filters on multiple properties: %q and %q",
						q.ineqFilter.prop, prop)
				}
				if len(q.order) > 0 && q.order[0].Property != prop {
					return fmt.Errorf(
						"first sort order must match inequality filter: %q v %q",
						q.order[0].Property, prop)
				}
			} else {
				for _, p := range q.project {
					if p == prop {
						return fmt.Errorf(
							"cannot project on field which is used in an equality filter: %q",
							prop)
					}
				}
			}
			return err
		},
		func(q *queryImpl) {
			if op == qEqual {
				// add it to eq filters
				q.addEqFilt(prop, p)

				// remove it from sort orders.
				// https://cloud.google.com/appengine/docs/go/datastore/queries#sort_orders_are_ignored_on_properties_with_equality_filters
				toRm := -1
				for i, o := range q.order {
					if o.Property == prop {
						toRm = i
						break
					}
				}
				if toRm >= 0 {
					q.order = append(q.order[:toRm], q.order[toRm+1:]...)
				}
			} else {
				q.ineqFilter.prop = prop
				if q.ineqFilter.constrain(op, serialize.ToBytes(p)) {
					q.err = errQueryDone
				}
			}
		})
}

func (q *queryImpl) Order(prop string) ds.Query {
	col := ds.IndexColumn{}
	return q.checkMutateClone(
		func() error {
			// check that first order == first inequality.
			// if order is an equality already, ignore it
			col.Property = strings.TrimSpace(prop)
			if strings.HasPrefix(prop, "-") {
				col.Direction = ds.DESCENDING
				col.Property = strings.TrimSpace(prop[1:])
			} else if strings.HasPrefix(prop, "+") {
				return fmt.Errorf("datastore: invalid order: %q", prop)
			}
			if len(col.Property) == 0 {
				return errors.New("datastore: empty order")
			}
			if len(q.order) == 0 && q.ineqFilter.prop != "" && q.ineqFilter.prop != col.Property {
				return fmt.Errorf(
					"first sort order must match inequality filter: %q v %q",
					prop, q.ineqFilter.prop)
			}
			if q.kind == "" && (col.Property != "__key__" || col.Direction != ds.ASCENDING) {
				return fmt.Errorf("invalid order for kindless query: %#v", col)
			}
			return nil
		},
		func(q *queryImpl) {
			if _, ok := q.eqFilters[col.Property]; ok {
				// skip it if it's an equality filter
				// https://cloud.google.com/appengine/docs/go/datastore/queries#sort_orders_are_ignored_on_properties_with_equality_filters
				return
			}
			for _, order := range q.order {
				if order.Property == col.Property {
					// can't sort by the same order twice
					return
				}
			}
			q.order = append(q.order, col)
		})
}

func (q *queryImpl) Project(fieldName ...string) ds.Query {
	return q.checkMutateClone(
		func() error {
			if q.keysOnly {
				return errors.New("cannot project a keysOnly query")
			}
			dupCheck := stringSet{}
			for _, f := range fieldName {
				if !dupCheck.add(f) {
					return fmt.Errorf("cannot project on the same field twice: %q", f)
				}
				if f == "" {
					return errors.New("cannot project on an empty field name")
				}
				if f == "__key__" {
					return fmt.Errorf("cannot project on __key__")
				}
				if _, ok := q.eqFilters[f]; ok {
					return fmt.Errorf(
						"cannot project on field which is used in an equality filter: %q", f)
				}
				for _, p := range q.project {
					if p == f {
						return fmt.Errorf("cannot project on the same field twice: %q", f)
					}
				}
			}
			return nil
		},
		func(q *queryImpl) {
			q.project = append(q.project, fieldName...)
		})
}

func (q *queryImpl) KeysOnly() ds.Query {
	return q.checkMutateClone(
		func() error {
			if len(q.project) != 0 {
				return errors.New("cannot project a keysOnly query")
			}
			return nil
		},
		func(q *queryImpl) {
			q.keysOnly = true
		})
}

func (q *queryImpl) Limit(limit int) ds.Query {
	return q.checkMutateClone(
		func() error {
			// nonsensically... ANY negative value means 'unlimited'. *shakes head*
			if limit < math.MinInt32 || limit > math.MaxInt32 {
				return errors.New("datastore: query limit overflow")
			}
			return nil
		},
		func(q *queryImpl) {
			q.limitSet = true
			q.limit = int32(limit)
		})
}

func (q *queryImpl) Offset(offset int) ds.Query {
	return q.checkMutateClone(
		func() error {
			if offset < 0 {
				return errors.New("datastore: negative query offset")
			}
			if offset > math.MaxInt32 {
				return errors.New("datastore: query offset overflow")
			}
			return nil
		},
		func(q *queryImpl) {
			q.offset = int32(offset)
		})
}

func queryCursorCheck(ns, flavor string, current []byte, newCursor ds.Cursor) ([]ds.IndexColumn, []byte, error) {
	if current != nil {
		return nil, nil, fmt.Errorf("%s cursor is multiply defined", flavor)
	}
	curs, ok := newCursor.(queryCursor)
	if !ok {
		return nil, nil, fmt.Errorf("%s cursor is unknown type: %T", flavor, curs)
	}
	return curs.decode()
}

func (q *queryImpl) Start(c ds.Cursor) ds.Query {
	cols := []ds.IndexColumn(nil)
	curs := []byte(nil)
	return q.checkMutateClone(
		func() (err error) {
			cols, curs, err = queryCursorCheck(q.ns, "start", q.startCursor, c)
			return
		},
		func(q *queryImpl) {
			q.startCursorColumns = cols
			q.startCursor = curs
		})
}

func (q *queryImpl) End(c ds.Cursor) ds.Query {
	cols := []ds.IndexColumn(nil)
	curs := queryCursor(nil)
	return q.checkMutateClone(
		func() (err error) {
			cols, curs, err = queryCursorCheck(q.ns, "end", q.endCursor, c)
			return
		},
		func(q *queryImpl) {
			q.endCursorColumns = cols
			q.endCursor = curs
		})
}

func (q *queryImpl) EventualConsistency() ds.Query {
	return q.checkMutateClone(
		nil, func(q *queryImpl) {
			q.eventualConsistency = true
		})
}
