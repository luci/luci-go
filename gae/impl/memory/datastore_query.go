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
	"github.com/luci/gkvlite"
)

type queryOp int

const (
	qInvalid queryOp = iota
	qEqual
	qLessThan
	qLessEq
	qGreaterEq
	qGreaterThan
)

func (o queryOp) isEQOp() bool {
	return o == qEqual
}

func (o queryOp) isINEQOp() bool {
	return o >= qLessThan && o <= qGreaterThan
}

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

func parseFilter(f string, v interface{}) (ret queryFilter, err error) {
	toks := strings.SplitN(strings.TrimSpace(f), " ", 2)
	if len(toks) != 2 {
		err = errors.New("datastore: invalid filter: " + f)
	} else {
		op := queryOpMap[toks[1]]
		if op == qInvalid {
			err = fmt.Errorf("datastore: invalid operator %q in filter %q", toks[1], f)
		} else {
			ret.prop = toks[0]
			ret.op = op
			ret.value = v
		}
	}
	return
}

type queryCursor string

func (q queryCursor) String() string { return string(q) }
func (q queryCursor) Valid() bool    { return q != "" }

type queryImpl struct {
	ns string

	kind     string
	ancestor ds.Key
	filter   []queryFilter
	order    []ds.IndexColumn
	project  []string

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

func (q *queryImpl) normalize() (ret *queryImpl) {
	// ported from GAE SDK datastore_index.py;Normalize()
	ret = q.clone()

	bs := newMemStore()

	eqProperties := bs.MakePrivateCollection(nil)

	ineqProperties := bs.MakePrivateCollection(nil)

	for _, f := range ret.filter {
		// if we supported the IN operator, we would check to see if there were
		// multiple value operands here, but the go SDK doesn't support this.
		if f.op.isEQOp() {
			eqProperties.Set([]byte(f.prop), []byte{})
		} else if f.op.isINEQOp() {
			ineqProperties.Set([]byte(f.prop), []byte{})
		}
	}

	ineqProperties.VisitItemsAscend(nil, false, func(i *gkvlite.Item) bool {
		eqProperties.Delete(i.Key)
		return true
	})

	removeSet := bs.MakePrivateCollection(nil)
	eqProperties.VisitItemsAscend(nil, false, func(i *gkvlite.Item) bool {
		removeSet.Set(i.Key, []byte{})
		return true
	})

	newOrders := []ds.IndexColumn{}
	for _, o := range ret.order {
		if removeSet.Get([]byte(o.Property)) == nil {
			removeSet.Set([]byte(o.Property), []byte{})
			newOrders = append(newOrders, o)
		}
	}
	ret.order = newOrders

	// need to fix ret.filters if we ever support the EXISTS operator and/or
	// projections.
	//
	//   newFilters = []
	//   for f in ret.filters:
	//     if f.op != qExists:
	//       newFilters = append(newFilters, f)
	//     if !removeSet.Has(f.prop):
	//       removeSet.InsertNoReplace(f.prop)
	//       newFilters = append(newFilters, f)
	//
	// so ret.filters == newFilters becuase none of ret.filters has op == qExists
	//
	// then:
	//
	//   for prop in ret.project:
	//     if !removeSet.Has(prop):
	//       removeSet.InsertNoReplace(prop)
	//      ... make new EXISTS filters, add them to newFilters ...
	//   ret.filters = newFilters
	//
	// However, since we don't support projection queries, this is moot.

	if eqProperties.Get([]byte("__key__")) != nil {
		ret.order = []ds.IndexColumn{}
	}

	newOrders = []ds.IndexColumn{}
	for _, o := range ret.order {
		if o.Property == "__key__" {
			newOrders = append(newOrders, o)
			break
		}
		newOrders = append(newOrders, o)
	}
	ret.order = newOrders

	return
}

func (q *queryImpl) checkCorrectness(ns string, isTxn bool) (ret *queryImpl) {
	// ported from GAE SDK datastore_stub_util.py;CheckQuery()
	ret = q.clone()

	if ns != ret.ns {
		ret.err = errors.New(
			"gae/memory: Namespace mismatched. Query and Datastore don't agree " +
				"on the current namespace")
		return
	}

	if ret.err != nil {
		return
	}

	// if projection && keys_only:
	//   "projection and keys_only cannot both be set"

	// if projection props match /^__.*__$/:
	//   "projections are not supported for the property: %(prop)s"

	if isTxn && ret.ancestor == nil {
		ret.err = errors.New(
			"gae/memory: Only ancestor queries are allowed inside transactions")
		return
	}

	numComponents := len(ret.filter) + len(ret.order)
	if ret.ancestor != nil {
		numComponents++
	}
	if numComponents > 100 {
		ret.err = errors.New(
			"gae/memory: query is too large. may not have more than " +
				"100 filters + sort orders ancestor total")
	}

	// if ret.ancestor.appid() != current appid
	//   "query app is x but ancestor app is x"
	// if ret.ancestor.namespace() != current namespace
	//   "query namespace is x but ancestor namespace is x"

	// if not all(g in orders for g in group_by)
	//  "items in the group by clause must be specified first in the ordering"

	ineqPropName := ""
	for _, f := range ret.filter {
		if f.prop == "__key__" {
			k, ok := f.value.(ds.Key)
			if !ok {
				ret.err = errors.New(
					"gae/memory: __key__ filter value must be a Key")
				return
			}
			if !ds.KeyValid(k, false, globalAppID, q.ns) {
				// See the comment in queryImpl.Ancestor; basically this check
				// never happens in the real env because the SDK silently swallows
				// this condition :/
				ret.err = ds.ErrInvalidKey
				return
			}
			if k.Namespace() != ns {
				ret.err = fmt.Errorf("bad namespace: %q (expected %q)", k.Namespace(), ns)
				return
			}
			// __key__ filter app is X but query app is X
			// __key__ filter namespace is X but query namespace is X
		}
		// if f.op == qEqual and f.prop in ret.project_fields
		//   "cannot use projection on a proprety with an equality filter"

		if f.op.isINEQOp() {
			if ineqPropName == "" {
				ineqPropName = f.prop
			} else if f.prop != ineqPropName {
				ret.err = fmt.Errorf(
					"gae/memory: Only one inequality filter per query is supported. "+
						"Encountered both %s and %s", ineqPropName, f.prop)
				return
			}
		}
	}

	// if ineqPropName != "" && len(group_by) > 0 && len(orders) ==0
	//   "Inequality filter on X must also be a group by property "+
	//   "when group by properties are set."

	if ineqPropName != "" && len(ret.order) != 0 {
		if ret.order[0].Property != ineqPropName {
			ret.err = fmt.Errorf(
				"gae/memory: The first sort property must be the same as the property "+
					"to which the inequality filter is applied.  In your query "+
					"the first sort property is %s but the inequality filter "+
					"is on %s", ret.order[0].Property, ineqPropName)
			return
		}
	}

	if ret.kind == "" {
		for _, f := range ret.filter {
			if f.prop != "__key__" {
				ret.err = errors.New(
					"gae/memory: kind is required for non-__key__ filters")
				return
			}
		}
		for _, o := range ret.order {
			if o.Property != "__key__" || o.Direction != ds.ASCENDING {
				ret.err = errors.New(
					"gae/memory: kind is required for all orders except __key__ ascending")
				return
			}
		}
	}
	return
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

func (q *queryImpl) clone() *queryImpl {
	ret := *q
	ret.filter = append([]queryFilter(nil), q.filter...)
	ret.order = append([]ds.IndexColumn(nil), q.order...)
	ret.project = append([]string(nil), q.project...)
	return &ret
}

func (q *queryImpl) Ancestor(k ds.Key) ds.Query {
	q = q.clone()
	q.ancestor = k
	if k == nil {
		// SDK has an explicit nil-check
		q.err = errors.New("datastore: nil query ancestor")
	} else if !ds.KeyValid(k, false, globalAppID, q.ns) {
		// technically the SDK implementation does a Weird Thing (tm) if both the
		// stringID and intID are set on a key; it only serializes the stringID in
		// the proto. This means that if you set the Ancestor to an invalid key,
		// you'll never actually hear about it. Instead of doing that insanity, we
		// just swap to an error here.
		q.err = ds.ErrInvalidKey
	} else if k.Namespace() != q.ns {
		q.err = fmt.Errorf("bad namespace: %q (expected %q)", k.Namespace(), q.ns)
	}
	return q
}

func (q *queryImpl) Distinct() ds.Query {
	q = q.clone()
	q.distinct = true
	return q
}

func (q *queryImpl) Filter(fStr string, val interface{}) ds.Query {
	q = q.clone()
	f, err := parseFilter(fStr, val)
	if err != nil {
		q.err = err
		return q
	}
	q.filter = append(q.filter, f)
	return q
}

func (q *queryImpl) Order(prop string) ds.Query {
	q = q.clone()
	prop = strings.TrimSpace(prop)
	o := ds.IndexColumn{Property: prop}
	if strings.HasPrefix(prop, "-") {
		o.Direction = ds.DESCENDING
		o.Property = strings.TrimSpace(prop[1:])
	} else if strings.HasPrefix(prop, "+") {
		q.err = fmt.Errorf("datastore: invalid order: %q", prop)
		return q
	}
	if len(o.Property) == 0 {
		q.err = errors.New("datastore: empty order")
		return q
	}
	q.order = append(q.order, o)
	return q
}

func (q *queryImpl) Project(fieldName ...string) ds.Query {
	q = q.clone()
	q.project = append(q.project, fieldName...)
	return q
}

func (q *queryImpl) KeysOnly() ds.Query {
	q = q.clone()
	q.keysOnly = true
	return q
}

func (q *queryImpl) Limit(limit int) ds.Query {
	q = q.clone()
	if limit < math.MinInt32 || limit > math.MaxInt32 {
		q.err = errors.New("datastore: query limit overflow")
		return q
	}
	q.limit = int32(limit)
	return q
}

func (q *queryImpl) Offset(offset int) ds.Query {
	q = q.clone()
	if offset < 0 {
		q.err = errors.New("datastore: negative query offset")
		return q
	}
	if offset > math.MaxInt32 {
		q.err = errors.New("datastore: query offset overflow")
		return q
	}
	q.offset = int32(offset)
	return q
}

func (q *queryImpl) Start(c ds.Cursor) ds.Query {
	q = q.clone()
	curs := c.(queryCursor)
	if !curs.Valid() {
		q.err = errors.New("datastore: invalid cursor")
		return q
	}
	q.start = curs
	return q
}

func (q *queryImpl) End(c ds.Cursor) ds.Query {
	q = q.clone()
	curs := c.(queryCursor)
	if !curs.Valid() {
		q.err = errors.New("datastore: invalid cursor")
		return q
	}
	q.end = curs
	return q
}

func (q *queryImpl) EventualConsistency() ds.Query {
	q = q.clone()
	q.eventualConsistency = true
	return q
}
