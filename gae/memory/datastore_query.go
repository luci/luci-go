// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"strings"

	"infra/gae/libs/gae"
	"infra/gae/libs/gae/helper"

	"github.com/luci/gkvlite"
	"github.com/luci/luci-go/common/cmpbin"
)

type qDirection bool

const (
	qASC qDirection = true
	qDEC            = false
)

var builtinQueryPrefix = []byte{0}
var complexQueryPrefix = []byte{1}

type qSortBy struct {
	prop string
	dir  qDirection
}

func (q qSortBy) WriteBinary(buf *bytes.Buffer) {
	if q.dir == qASC {
		buf.WriteByte(0)
	} else {
		buf.WriteByte(1)
	}
	cmpbin.WriteString(buf, q.prop)
}

func (q *qSortBy) ReadBinary(buf *bytes.Buffer) error {
	dir, err := buf.ReadByte()
	if err != nil {
		return err
	}
	q.dir = dir == 0
	q.prop, _, err = cmpbin.ReadString(buf)
	return err
}

type qIndex struct {
	kind     string
	ancestor bool
	sortby   []qSortBy
}

func (i *qIndex) Builtin() bool {
	return !i.ancestor && len(i.sortby) <= 1
}

func (i *qIndex) Less(o *qIndex) bool {
	ibuf, obuf := &bytes.Buffer{}, &bytes.Buffer{}
	i.WriteBinary(ibuf)
	o.WriteBinary(obuf)
	return i.String() < o.String()
}

// Valid verifies that this qIndex doesn't have duplicate sortBy fields.
func (i *qIndex) Valid() bool {
	names := map[string]bool{}
	for _, sb := range i.sortby {
		if names[sb.prop] {
			return false
		}
		names[sb.prop] = true
	}
	return true
}

func (i *qIndex) WriteBinary(buf *bytes.Buffer) {
	// TODO(riannucci): do a Grow call here?
	if i.Builtin() {
		buf.Write(builtinQueryPrefix)
	} else {
		buf.Write(complexQueryPrefix)
	}
	cmpbin.WriteString(buf, i.kind)
	if i.ancestor {
		buf.WriteByte(0)
	} else {
		buf.WriteByte(1)
	}
	cmpbin.WriteUint(buf, uint64(len(i.sortby)))
	for _, sb := range i.sortby {
		sb.WriteBinary(buf)
	}
}

func (i *qIndex) String() string {
	ret := &bytes.Buffer{}
	if i.Builtin() {
		ret.WriteRune('B')
	} else {
		ret.WriteRune('C')
	}
	ret.WriteRune(':')
	ret.WriteString(i.kind)
	if i.ancestor {
		ret.WriteString("|A")
	}
	for _, sb := range i.sortby {
		ret.WriteRune('/')
		if sb.dir == qDEC {
			ret.WriteRune('-')
		}
		ret.WriteString(sb.prop)
	}
	return ret.String()
}

func (i *qIndex) ReadBinary(buf *bytes.Buffer) error {
	// discard builtin/complex byte
	_, err := buf.ReadByte()
	if err != nil {
		return err
	}

	i.kind, _, err = cmpbin.ReadString(buf)
	if err != nil {
		return err
	}
	anc, err := buf.ReadByte()
	if err != nil {
		return err
	}
	i.ancestor = anc == 1

	numSorts, _, err := cmpbin.ReadUint(buf)
	if err != nil {
		return err
	}
	if numSorts > 64 {
		return fmt.Errorf("qIndex.ReadBinary: Got over 64 sort orders: %d", numSorts)
	}
	i.sortby = make([]qSortBy, numSorts)
	for idx := range i.sortby {
		err = (&i.sortby[idx]).ReadBinary(buf)
		if err != nil {
			return err
		}
	}

	return nil
}

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
	field string
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
			ret.field = toks[0]
			ret.op = op
			ret.value = v
		}
	}
	return
}

type queryOrder struct {
	field     string
	direction qDirection
}

type queryCursor string

func (q queryCursor) String() string { return string(q) }
func (q queryCursor) Valid() bool    { return q != "" }

type queryImpl struct {
	gae.DSQuery

	ns string

	kind     string
	ancestor gae.DSKey
	filter   []queryFilter
	order    []queryOrder

	keysOnly bool
	limit    int32
	offset   int32

	start queryCursor
	end   queryCursor

	err error
}

type queryIterImpl struct {
	idx *queryImpl
}

var _ gae.RDSIterator = (*queryIterImpl)(nil)

func (q *queryIterImpl) Cursor() (gae.DSCursor, error) {
	if q.idx.err != nil {
		return nil, q.idx.err
	}
	return nil, nil
}

func (q *queryIterImpl) Next(dst gae.DSPropertyLoadSaver) (gae.DSKey, error) {
	if q.idx.err != nil {
		return nil, q.idx.err
	}
	return nil, nil
}

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
			eqProperties.Set([]byte(f.field), []byte{})
		} else if f.op.isINEQOp() {
			ineqProperties.Set([]byte(f.field), []byte{})
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

	newOrders := []queryOrder{}
	for _, o := range ret.order {
		if removeSet.Get([]byte(o.field)) == nil {
			removeSet.Set([]byte(o.field), []byte{})
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
	//     if !removeSet.Has(f.field):
	//       removeSet.InsertNoReplace(f.field)
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
		ret.order = []queryOrder{}
	}

	newOrders = []queryOrder{}
	for _, o := range ret.order {
		if o.field == "__key__" {
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
		if f.field == "__key__" {
			k, ok := f.value.(gae.DSKey)
			if !ok {
				ret.err = errors.New(
					"gae/memory: __key__ filter value must be a Key")
				return
			}
			if !helper.DSKeyValid(k, ret.ns, false) {
				// See the comment in queryImpl.Ancestor; basically this check
				// never happens in the real env because the SDK silently swallows
				// this condition :/
				ret.err = gae.ErrDSInvalidKey
				return
			}
			// __key__ filter app is X but query app is X
			// __key__ filter namespace is X but query namespace is X
		}
		// if f.op == qEqual and f.field in ret.project_fields
		//   "cannot use projection on a proprety with an equality filter"

		if f.op.isINEQOp() {
			if ineqPropName == "" {
				ineqPropName = f.field
			} else if f.field != ineqPropName {
				ret.err = fmt.Errorf(
					"gae/memory: Only one inequality filter per query is supported. "+
						"Encountered both %s and %s", ineqPropName, f.field)
				return
			}
		}
	}

	// if ineqPropName != "" && len(group_by) > 0 && len(orders) ==0
	//   "Inequality filter on X must also be a group by property "+
	//   "when group by properties are set."

	if ineqPropName != "" && len(ret.order) != 0 {
		if ret.order[0].field != ineqPropName {
			ret.err = fmt.Errorf(
				"gae/memory: The first sort property must be the same as the property "+
					"to which the inequality filter is applied.  In your query "+
					"the first sort property is %s but the inequality filter "+
					"is on %s", ret.order[0].field, ineqPropName)
			return
		}
	}

	if ret.kind == "" {
		for _, f := range ret.filter {
			if f.field != "__key__" {
				ret.err = errors.New(
					"gae/memory: kind is required for non-__key__ filters")
				return
			}
		}
		for _, o := range ret.order {
			if o.field != "__key__" || o.direction != qASC {
				ret.err = errors.New(
					"gae/memory: kind is required for all orders except __key__ ascending")
				return
			}
		}
	}
	return
}

func (q *queryImpl) calculateIndex() *qIndex {
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
	ret.order = append([]queryOrder(nil), q.order...)
	return &ret
}

func (q *queryImpl) Ancestor(k gae.DSKey) gae.DSQuery {
	q = q.clone()
	q.ancestor = k
	if k == nil {
		// SDK has an explicit nil-check
		q.err = errors.New("datastore: nil query ancestor")
	} else if !helper.DSKeyValid(k, q.ns, false) {
		// technically the SDK implementation does a Weird Thing (tm) if both the
		// stringID and intID are set on a key; it only serializes the stringID in
		// the proto. This means that if you set the Ancestor to an invalid key,
		// you'll never actually hear about it. Instead of doing that insanity, we
		// just swap to an error here.
		q.err = gae.ErrDSInvalidKey
	}
	return q
}

func (q *queryImpl) Filter(fStr string, val interface{}) gae.DSQuery {
	q = q.clone()
	f, err := parseFilter(fStr, val)
	if err != nil {
		q.err = err
		return q
	}
	q.filter = append(q.filter, f)
	return q
}

func (q *queryImpl) Order(field string) gae.DSQuery {
	q = q.clone()
	field = strings.TrimSpace(field)
	o := queryOrder{field, qASC}
	if strings.HasPrefix(field, "-") {
		o.direction = qDEC
		o.field = strings.TrimSpace(field[1:])
	} else if strings.HasPrefix(field, "+") {
		q.err = fmt.Errorf("datastore: invalid order: %q", field)
		return q
	}
	if len(o.field) == 0 {
		q.err = errors.New("datastore: empty order")
		return q
	}
	q.order = append(q.order, o)
	return q
}

func (q *queryImpl) KeysOnly() gae.DSQuery {
	q = q.clone()
	q.keysOnly = true
	return q
}

func (q *queryImpl) Limit(limit int) gae.DSQuery {
	q = q.clone()
	if limit < math.MinInt32 || limit > math.MaxInt32 {
		q.err = errors.New("datastore: query limit overflow")
		return q
	}
	q.limit = int32(limit)
	return q
}

func (q *queryImpl) Offset(offset int) gae.DSQuery {
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

func (q *queryImpl) Start(c gae.DSCursor) gae.DSQuery {
	q = q.clone()
	curs := c.(queryCursor)
	if !curs.Valid() {
		q.err = errors.New("datastore: invalid cursor")
		return q
	}
	q.start = curs
	return q
}

func (q *queryImpl) End(c gae.DSCursor) gae.DSQuery {
	q = q.clone()
	curs := c.(queryCursor)
	if !curs.Valid() {
		q.err = errors.New("datastore: invalid cursor")
		return q
	}
	q.end = curs
	return q
}
