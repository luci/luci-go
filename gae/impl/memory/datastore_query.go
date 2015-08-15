// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"errors"
	"fmt"
	"math"
	"strings"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/datastore/serialize"
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

type queryCursor string

func (q queryCursor) String() string { return string(q) }
func (q queryCursor) Valid() bool    { return q != "" }

type queryIneqFilter struct {
	prop string

	low  *string
	high *string
}

func increment(bstr string, positive bool) string {
	lastIdx := len(bstr) - 1
	last := bstr[lastIdx]
	if positive {
		if last == 0xFF {
			return bstr + "\x00"
		}
		return bstr[:lastIdx-1] + string(last+1)
	} else {
		if last == 0 {
			return bstr[:lastIdx-1]
		}
		return bstr[:lastIdx-1] + string(last-1)
	}
}

// constrain 'folds' a new inequality into the current inequality filter.
//
// It will bump the high bound down, or the low bound up, assuming the incoming
// constraint does so.
//
// It returns true iff the filter is overconstrained (i.e.  low > high)
func (q *queryIneqFilter) constrain(op queryOp, val string) bool {
	switch op {
	case qLessThan:
		val = increment(val, true)
		fallthrough
	case qLessEq:
		// adjust upper bound downwards
		if q.high == nil || *q.high > val {
			q.high = &val
		}

	case qGreaterThan:
		val = increment(val, false)
		fallthrough
	case qGreaterEq:
		// adjust lower bound upwards
		if q.low == nil || *q.low < val {
			q.low = &val
		}

	default:
		panic(fmt.Errorf("constrain cannot handle filter op %d", op))
	}

	if q.low != nil && q.high != nil {
		return *q.low > *q.high
	}
	return false
}

type queryImpl struct {
	ns string

	kind     string
	ancestor ds.Key

	// prop -> encoded values
	eqFilters  map[string]map[string]struct{}
	ineqFilter queryIneqFilter
	order      []ds.IndexColumn
	project    map[string]struct{}

	distinct            bool
	eventualConsistency bool
	keysOnly            bool
	limit               int32
	offset              int32

	start queryCursor
	end   queryCursor

	err error
}

var _ ds.Query = (*queryImpl)(nil)

func (q *queryImpl) valid(ns string, isTxn bool) (done bool, err error) {
	if q.err == errQueryDone {
		done = true
	} else if q.err != nil {
		err = q.err
	} else if ns != q.ns {
		err = errors.New(
			"gae/memory: Namespace mismatched. Query and Datastore don't agree " +
				"on the current namespace")
	} else if isTxn && q.ancestor == nil {
		err = errors.New(
			"gae/memory: Only ancestor queries are allowed inside transactions")
	} else if q.numComponents() > MaxQueryComponents {
		err = fmt.Errorf(
			"gae/memory: query is too large. may not have more than "+
				"%d filters + sort orders + ancestor total: had %d",
			MaxQueryComponents, q.numComponents())
	} else if len(q.project) == 0 && q.distinct {
		// This must be delayed, because q.Distinct().Project("foo") is a valid
		// construction. If we checked this in Distinct, it could be too early, and
		// checking it in Project doesn't matter.
		err = errors.New(
			"gae/memory: Distinct() only makes sense on projection queries.")
	}
	return
}

func (q *queryImpl) numComponents() int {
	numComponents := len(q.order)
	if q.ineqFilter.prop != "" {
		if q.ineqFilter.low != nil {
			numComponents++
		}
		if q.ineqFilter.high != nil {
			numComponents++
		}
	}
	for _, v := range q.eqFilters {
		numComponents += len(v)
	}
	if q.ancestor != nil {
		numComponents++
	}
	return numComponents
}

func (q *queryImpl) calculateIndex() *ds.IndexDefinition {
	// as a nod to simplicity in this code, we'll require that a single index
	// is able to service the entire query. E.g. no zigzag merge joins or
	// multiqueries. This will mean that the user will need to rely on
	// dev_appserver to tell them what indicies they need for real, and for thier
	// tests they'll need to specify the missing composite indices manually.
	//
	// This COULD lead to an exploding indicies problem, but we can fix that when
	// we get to it.

	//sortOrders := []qSortBy{}

	return nil
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
	nq.eqFilters = make(map[string]map[string]struct{}, len(q.eqFilters))
	for prop, vals := range q.eqFilters {
		nq.eqFilters[prop] = make(map[string]struct{}, len(vals))
		for v := range vals {
			nq.eqFilters[prop][v] = struct{}{}
		}
	}
	nq.order = make([]ds.IndexColumn, len(q.order))
	copy(nq.order, q.order)
	nq.project = make(map[string]struct{}, len(q.project))
	for f := range q.project {
		nq.project[f] = struct{}{}
	}
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
			if !k.Valid(false, globalAppID, q.ns) {
				// technically the SDK implementation does a Weird Thing (tm) if both the
				// stringID and intID are set on a key; it only serializes the stringID in
				// the proto. This means that if you set the Ancestor to an invalid key,
				// you'll never actually hear about it. Instead of doing that insanity, we
				// just swap to an error here.
				return ds.ErrInvalidKey
			}
			if k.Namespace() != q.ns {
				return fmt.Errorf("bad namespace: %q (expected %q)", k.Namespace(), q.ns)
			}
			if q.ancestor != nil {
				return errors.New("cannot have more than one ancestor")
			}
			return nil
		},
		func(q *queryImpl) {
			q.ancestor = k
		})
}

func (q *queryImpl) Distinct() ds.Query {
	return q.checkMutateClone(nil, func(q *queryImpl) {
		q.distinct = true
	})
}

func (q *queryImpl) Filter(fStr string, val interface{}) ds.Query {
	prop := ""
	op := qInvalid
	binVal := ""
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

			p := ds.Property{}
			err = p.SetValue(val, ds.NoIndex)
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
			} else if _, ok := q.project[prop]; ok {
				return fmt.Errorf(
					"cannot project on field which is used in an equality filter: %q",
					prop)
			}
			binVal = string(serialize.ToBytes(p))
			return err
		},
		func(q *queryImpl) {
			if op == qEqual {
				// add it to eq filters
				if _, ok := q.eqFilters[prop]; !ok {
					q.eqFilters[prop] = map[string]struct{}{binVal: {}}
				} else {
					q.eqFilters[prop][binVal] = struct{}{}
				}

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
				if q.ineqFilter.constrain(op, binVal) {
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
			if q.ineqFilter.prop != "" && q.ineqFilter.prop != col.Property {
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
			if col.Property == "__key__" {
				// __key__ order dominates all other orders
				q.order = []ds.IndexColumn{col}
			} else {
				q.order = append(q.order, col)
			}
		})
}

func (q *queryImpl) Project(fieldName ...string) ds.Query {
	return q.checkMutateClone(
		func() error {
			if q.keysOnly {
				return errors.New("cannot project a keysOnly query")
			}
			for _, f := range fieldName {
				if f == "" {
					return errors.New("cannot project on an empty field name")
				}
				if strings.HasPrefix(f, "__") && strings.HasSuffix(f, "__") {
					return fmt.Errorf("cannot project on %q", f)
				}
				if _, ok := q.eqFilters[f]; ok {
					return fmt.Errorf(
						"cannot project on field which is used in an equality filter: %q", f)
				}
			}
			return nil
		},
		func(q *queryImpl) {
			for _, f := range fieldName {
				q.project[f] = struct{}{}
			}
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
			if limit < math.MinInt32 || limit > math.MaxInt32 {
				return errors.New("datastore: query limit overflow")
			}
			return nil
		},
		func(q *queryImpl) {
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

func (q *queryImpl) Start(c ds.Cursor) ds.Query {
	curs := queryCursor("")
	return q.checkMutateClone(
		func() error {
			ok := false
			if curs, ok = c.(queryCursor); !ok {
				return fmt.Errorf("start cursor is unknown type: %T", c)
			}
			if !curs.Valid() {
				return errors.New("datastore: invalid cursor")
			}
			return nil
		},
		func(q *queryImpl) {
			q.start = curs
		})
}

func (q *queryImpl) End(c ds.Cursor) ds.Query {
	curs := queryCursor("")
	return q.checkMutateClone(
		func() error {
			ok := false
			if curs, ok = c.(queryCursor); !ok {
				return fmt.Errorf("end cursor is unknown type: %T", c)
			}
			if !curs.Valid() {
				return errors.New("datastore: invalid cursor")
			}
			return nil
		},
		func(q *queryImpl) {
			q.end = curs
		})
}

func (q *queryImpl) EventualConsistency() ds.Query {
	return q.checkMutateClone(
		nil, func(q *queryImpl) {
			q.eventualConsistency = true
		})
}
