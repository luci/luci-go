// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"bytes"
	"fmt"
	"reflect"
	"sort"
	"time"

	"appengine"
	"appengine/datastore"

	"github.com/luci/gkvlite"
	"github.com/luci/luci-go/common/cmpbin"
)

type typData struct {
	noIndex bool
	typ     propValType
	data    interface{}
}

func newTypData(noIndex bool, v interface{}) (ret *typData, err error) {
	typ := pvUNKNOWN

	switch x := v.(type) {
	case nil:
		typ = pvNull
	case time.Time:
		typ = pvTime
	case int64:
		typ = pvInt
	case float64:
		typ = pvFloat
	case bool:
		if x {
			typ = pvBoolTrue
		} else {
			typ = pvBoolFalse
		}
	case []byte, datastore.ByteString:
		typ = pvBytes
	case appengine.BlobKey:
		typ = pvBlobKey
	case string:
		typ = pvStr
	case appengine.GeoPoint:
		typ = pvGeoPoint
	case *datastore.Key:
		typ = pvKey
	}
	if typ == pvUNKNOWN {
		err = fmt.Errorf("propValTypeOf: unknown type of %T: %#v", v, v)
	}

	return &typData{noIndex, typ, v}, err
}

func (td *typData) WriteBinary(buf *bytes.Buffer, nso nsOption) error {
	typb := byte(td.typ)
	if td.noIndex {
		typb |= 0x80
	}
	buf.WriteByte(typb)
	switch td.typ {
	case pvNull, pvBoolFalse, pvBoolTrue:
		return nil
	case pvInt:
		cmpbin.WriteInt(buf, td.data.(int64))
	case pvFloat:
		writeFloat64(buf, td.data.(float64))
	case pvStr:
		writeString(buf, td.data.(string))
	case pvBytes:
		if td.noIndex {
			writeBytes(buf, td.data.([]byte))
		} else {
			writeBytes(buf, td.data.(datastore.ByteString))
		}
	case pvTime:
		writeTime(buf, td.data.(time.Time))
	case pvGeoPoint:
		writeGeoPoint(buf, td.data.(appengine.GeoPoint))
	case pvKey:
		writeKey(buf, nso, td.data.(*datastore.Key))
	case pvBlobKey:
		writeString(buf, string(td.data.(appengine.BlobKey)))
	default:
		return fmt.Errorf("write: unknown type! %v", td)
	}
	return nil
}

func (td *typData) ReadBinary(buf *bytes.Buffer, nso nsOption, ns string) error {
	typb, err := buf.ReadByte()
	if err != nil {
		return err
	}
	td.noIndex = (typb & 0x80) != 0 // highbit means noindex
	td.typ = propValType(typb & 0x7f)
	switch td.typ {
	case pvNull:
		td.data = nil
	case pvBoolTrue:
		td.data = true
	case pvBoolFalse:
		td.data = false
	case pvInt:
		td.data, _, err = cmpbin.ReadInt(buf)
	case pvFloat:
		td.data, err = readFloat64(buf)
	case pvStr:
		td.data, err = readString(buf)
	case pvBytes:
		b := []byte(nil)
		if b, err = readBytes(buf); err != nil {
			return err
		}
		if td.noIndex {
			td.data = b
		} else {
			td.data = datastore.ByteString(b)
		}
	case pvTime:
		td.data, err = readTime(buf)
	case pvGeoPoint:
		td.data, err = readGeoPoint(buf)
	case pvKey:
		td.data, err = readKey(buf, nso, ns)
	case pvBlobKey:
		s := ""
		if s, err = readString(buf); err != nil {
			return err
		}
		td.data = appengine.BlobKey(s)
	default:
		return fmt.Errorf("read: unknown type! %v", td)
	}

	return err
}

type pvals struct {
	name string
	vals []*typData
}

type propertyList []datastore.Property

var _ = datastore.PropertyLoadSaver((*propertyList)(nil))

func (pl *propertyList) Load(ch <-chan datastore.Property) error {
	return (*datastore.PropertyList)(pl).Load(ch)
}

func (pl *propertyList) Save(ch chan<- datastore.Property) error {
	return (*datastore.PropertyList)(pl).Save(ch)
}

// collatedProperties is the reduction of a *propertyList such that each entry
// in a collatedProperties has a unique name. For example, collating this:
//   pl := &propertyList{
//     datastore.Property{Name: "wat", Val: "hello"},
//     datastore.Property{Name: "other", Val: 100},
//     datastore.Property{Name: "wat", Val: "goodbye", noIndex: true},
//   }
//
// Would get a collatedProperties which looked like:
//   c := collatedProperties{
//     &pvals{"wat",   []*typData{&{false, pvStr, "hello"},
//                                &{true,  pvStr, "goodbye"}}},
//     &pvals{"other", []*typData{&{false, pvInt, 100}}}
//   }
type collatedProperties []*pvals

func (c collatedProperties) defaultIndicies(kind string) []*qIndex {
	ret := make([]*qIndex, 0, 2*len(c)+1)
	ret = append(ret, &qIndex{kind, false, nil})
	for _, pvals := range c {
		needsIndex := false
		for _, v := range pvals.vals {
			if !v.noIndex {
				needsIndex = true
				break
			}
		}
		if !needsIndex {
			continue
		}
		ret = append(ret, &qIndex{kind, false, []qSortBy{{pvals.name, qASC}}})
		ret = append(ret, &qIndex{kind, false, []qSortBy{{pvals.name, qDEC}}})
	}
	return ret
}

// serializedPval is a single pvals.vals entry which has been serialized (in
// qASC order).
type serializedPval []byte

// serializedPvals is all of the pvals.vals entries from a single pvals (in qASC
// order). It does not include the pvals.name field.
type serializedPvals []serializedPval

func (s serializedPvals) Len() int           { return len(s) }
func (s serializedPvals) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s serializedPvals) Less(i, j int) bool { return bytes.Compare(s[i], s[j]) < 0 }

type mappedPlist map[string]serializedPvals

func (c collatedProperties) indexableMap() (mappedPlist, error) {
	ret := make(mappedPlist, len(c))
	for _, pv := range c {
		data := make(serializedPvals, 0, len(pv.vals))
		for _, v := range pv.vals {
			if v.noIndex {
				continue
			}
			buf := &bytes.Buffer{}
			if err := v.WriteBinary(buf, noNS); err != nil {
				return nil, err
			}
			data = append(data, buf.Bytes())
		}
		if len(data) == 0 {
			continue
		}
		sort.Sort(data)
		ret[pv.name] = data
	}
	return ret, nil
}

// indexRowGen contains enough information to generate all of the index rows which
// correspond with a propertyList and a qIndex.
type indexRowGen struct {
	propVec []serializedPvals
	orders  []qDirection
}

// permute calls cb for each index row, in the sorted order of the rows.
func (s indexRowGen) permute(cb func([]byte)) {
	iVec := make([]int, len(s.propVec))
	iVecLim := make([]int, len(s.propVec))

	incPos := func() bool {
		for i := len(iVec) - 1; i >= 0; i-- {
			var done bool
			var newVal int
			if s.orders[i] == qASC {
				newVal = (iVec[i] + 1) % iVecLim[i]
				done = newVal != 0
			} else {
				newVal = (iVec[i] - 1)
				if newVal < 0 {
					newVal = iVecLim[i] - 1
				} else {
					done = true
				}
			}
			iVec[i] = newVal
			if done {
				return true
			}
		}
		return false
	}

	for i, sps := range s.propVec {
		iVecLim[i] = len(sps)
	}

	for i := range iVec {
		if s.orders[i] == qDEC {
			iVec[i] = iVecLim[i] - 1
		}
	}

	for {
		bufsiz := 0
		for pvalSliceIdx, pvalIdx := range iVec {
			bufsiz += len(s.propVec[pvalSliceIdx][pvalIdx])
		}
		buf := bytes.NewBuffer(make([]byte, 0, bufsiz))
		for pvalSliceIdx, pvalIdx := range iVec {
			data := s.propVec[pvalSliceIdx][pvalIdx]
			if s.orders[pvalSliceIdx] == qASC {
				buf.Write(data)
			} else {
				for _, b := range data {
					buf.WriteByte(b ^ 0xFF)
				}
			}
		}
		cb(buf.Bytes())
		if !incPos() {
			break
		}
	}
}

type matcher struct {
	buf indexRowGen
}

// matcher.match checks to see if the mapped, serialized property values
// match the index. If they do, it returns a indexRowGen. Do not write or modify
// the data in the indexRowGen.
func (m *matcher) match(idx *qIndex, mpvals mappedPlist) (indexRowGen, bool) {
	m.buf.propVec = m.buf.propVec[:0]
	m.buf.orders = m.buf.orders[:0]
	for _, sb := range idx.sortby {
		if pv, ok := mpvals[sb.prop]; ok {
			m.buf.propVec = append(m.buf.propVec, pv)
			m.buf.orders = append(m.buf.orders, sb.dir)
		} else {
			return indexRowGen{}, false
		}
	}
	return m.buf, true
}

func (c collatedProperties) indexEntries(k *datastore.Key, idxs []*qIndex) (*memStore, error) {
	m, err := c.indexableMap()
	if err != nil {
		return nil, err
	}

	ret := newMemStore()
	idxColl := ret.SetCollection("idx", nil)
	// getIdxEnts retrieves an index collection or adds it if it's not there.
	getIdxEnts := func(qi *qIndex) *memCollection {
		buf := &bytes.Buffer{}
		qi.WriteBinary(buf)
		b := buf.Bytes()
		idxColl.Set(b, []byte{})
		return ret.SetCollection(fmt.Sprintf("idx:%s:%s", k.Namespace(), b), nil)
	}

	buf := &bytes.Buffer{}
	writeKey(buf, noNS, k) // ns is in idxEnts collection name.
	keyData := buf.Bytes()

	walkPermutations := func(prefix []byte, irg indexRowGen, ents *memCollection) {
		prev := []byte{} // intentionally make a non-nil slice, gkvlite hates nil.
		irg.permute(func(data []byte) {
			buf := bytes.NewBuffer(make([]byte, 0, len(prefix)+len(data)+len(keyData)))
			buf.Write(prefix)
			buf.Write(data)
			buf.Write(keyData)
			ents.Set(buf.Bytes(), prev)
			prev = data
		})
	}

	mtch := matcher{}
	for _, idx := range idxs {
		if irg, ok := mtch.match(idx, m); ok {
			idxEnts := getIdxEnts(idx)
			if len(irg.propVec) == 0 {
				idxEnts.Set(keyData, []byte{}) // propless index, e.g. kind -> key = nil
			} else if idx.ancestor {
				for ancKey := k; ancKey != nil; ancKey = ancKey.Parent() {
					buf := &bytes.Buffer{}
					writeKey(buf, noNS, ancKey)
					walkPermutations(buf.Bytes(), irg, idxEnts)
				}
			} else {
				walkPermutations(nil, irg, idxEnts)
			}
		}
	}

	return ret, nil
}

func (pl *propertyList) indexEntriesWithBuiltins(k *datastore.Key, complexIdxs []*qIndex) (ret *memStore, err error) {
	c, err := pl.collate()
	if err == nil {
		ret, err = c.indexEntries(k, append(c.defaultIndicies(k.Kind()), complexIdxs...))
	}
	return
}

func (pl *propertyList) collate() (collatedProperties, error) {
	if pl == nil || len(*pl) == 0 {
		return nil, nil
	}

	cols := []*pvals{}
	colIdx := map[string]int{}

	for _, p := range *pl {
		if idx, ok := colIdx[p.Name]; ok {
			c := cols[idx]
			td, err := newTypData(p.NoIndex, p.Value)
			if err != nil {
				return nil, err
			}
			c.vals = append(c.vals, td)
		} else {
			colIdx[p.Name] = len(cols)
			td, err := newTypData(p.NoIndex, p.Value)
			if err != nil {
				return nil, err
			}
			cols = append(cols, &pvals{p.Name, []*typData{td}})
		}
	}

	return cols, nil
}

func (pl *propertyList) addCollated(pv *pvals) {
	for _, v := range pv.vals {
		*pl = append(*pl, datastore.Property{
			Name:     pv.name,
			Multiple: len(pv.vals) > 1,
			NoIndex:  v.noIndex,
			Value:    v.data,
		})
	}
}

func updateIndicies(store *memStore, key *datastore.Key, oldEnt, newEnt *propertyList) error {
	var err error

	idxColl := store.GetCollection("idx")
	if idxColl == nil {
		idxColl = store.SetCollection("idx", nil)
	}

	// load all current complex query index definitions.
	compIdx := []*qIndex{}
	idxColl.VisitItemsAscend(complexQueryPrefix, false, func(i *gkvlite.Item) bool {
		if !bytes.HasPrefix(i.Key, complexQueryPrefix) {
			return false
		}
		qi := &qIndex{}
		if err = qi.ReadBinary(bytes.NewBuffer(i.Key)); err != nil {
			return false
		}
		compIdx = append(compIdx, qi)
		return true
	})
	if err != nil {
		return err
	}

	oldIdx, err := oldEnt.indexEntriesWithBuiltins(key, compIdx)
	if err != nil {
		return err
	}

	newIdx, err := newEnt.indexEntriesWithBuiltins(key, compIdx)
	if err != nil {
		return err
	}

	prefix := "idx:" + key.Namespace() + ":"

	gkvCollide(oldIdx.GetCollection("idx"), newIdx.GetCollection("idx"), func(k, ov, nv []byte) {
		ks := prefix + string(k)
		idxColl.Set(k, []byte{})

		coll := store.GetCollection(ks)
		if coll == nil {
			coll = store.SetCollection(ks, nil)
		}
		oldColl := oldIdx.GetCollection(ks)
		newColl := newIdx.GetCollection(ks)

		switch {
		case ov == nil && nv != nil: // all additions
			newColl.VisitItemsAscend(nil, false, func(i *gkvlite.Item) bool {
				coll.Set(i.Key, i.Val)
				return true
			})
		case ov != nil && nv == nil: // all deletions
			oldColl.VisitItemsAscend(nil, false, func(i *gkvlite.Item) bool {
				coll.Delete(i.Key)
				return true
			})
		case ov != nil && nv != nil: // merge
			gkvCollide(oldColl, newColl, func(k, ov, nv []byte) {
				if nv == nil {
					coll.Delete(k)
				} else {
					coll.Set(k, nv)
				}
			})
		default:
			panic("impossible")
		}
		// TODO(riannucci): remove entries from idxColl and remove index collections
		// when there are no index entries for that index any more.
	})

	return nil
}

func (pl *propertyList) MarshalBinary() ([]byte, error) {
	cols, err := pl.collate()
	if err != nil || len(cols) == 0 {
		return nil, err
	}

	pieces := make([][]byte, 0, len(*pl)*2+1)
	for _, pv := range cols {
		// TODO(riannucci): estimate buffer size better.
		buf := bytes.NewBuffer(make([]byte, 0, cmpbin.MaxIntLen64+len(pv.name)))
		writeString(buf, pv.name)
		err := pv.WriteBinary(buf)
		if err != nil {
			return nil, err
		}
		pieces = append(pieces, buf.Bytes())
	}
	return bytes.Join(pieces, nil), nil
}

func (pl *propertyList) UnmarshalBinary(data []byte) error {
	buf := bytes.NewBuffer(data)
	for buf.Len() > 0 {
		name, err := readString(buf)
		if err != nil {
			return err
		}

		pv := &pvals{name: name}
		err = pv.ReadBinary(buf)
		if err != nil {
			return err
		}
		pl.addCollated(pv)
	}

	return nil
}

func toPL(src interface{}) (ret *propertyList, err error) {
	propchan := make(chan datastore.Property)
	ret = &propertyList{}
	go func() { err = datastore.SaveStruct(src, propchan) }()
	err2 := ret.Load(propchan)
	if err != nil {
		return
	}
	return ret, err2
}

func fromPL(props *propertyList, dst interface{}) (err error) {
	propchan := make(chan datastore.Property)
	go func() { err = props.Save(propchan) }()
	err2 := datastore.LoadStruct(dst, propchan)
	if err != nil {
		return err
	}
	return err2
}

type propValType byte

var byteSliceType = reflect.TypeOf([]byte(nil))

// These constants are in the order described by
//   https://cloud.google.com/appengine/docs/go/datastore/entities#Go_Value_type_ordering
// with a slight divergence for the Int/Time split.
// NOTE: this enum can only occupy 7 bits, because we use the high bit to encode
// indexed/non-indexed. See typData.WriteBinary.
const (
	pvNull propValType = iota
	pvInt

	// NOTE: this is a slight divergence; times and integers actually sort
	// together (apparently?) in datastore. This is probably insane, and I don't
	// want to add the complexity of field 'meaning' as a sparate concept from the
	// field's 'type' (which is what datastore seems to do, judging from the
	// protobufs). So if you're here because you implemented an app which relies
	// on time.Time and int64 sorting together, then this is why your app acts
	// differently in production. My advice is to NOT DO THAT. If you really want
	// this (and you probably don't), you should take care of the time.Time <->
	// int64 conversion in your app and just use a property type of int64.
	pvTime

	// NOTE: this is also a slight divergence, but not a semantic one. IIUC, in
	// datastore 'bool' is actually the type and the value is either 0 or
	// 1 (taking another byte to store). Since we have plenty of space in this
	// type byte, I just merge the value into the type for booleans. If this
	// becomes problematic, consider changing this to just pvBool, and then
	// encoding a 0 or 1 as a byte in the relevant marshalling routines.
	pvBoolFalse
	pvBoolTrue
	pvBytes // []byte or datastore.ByteString
	pvStr   // string or string noindex
	pvFloat
	pvGeoPoint

	// These two are problematic, because they force us to bind to the appengine
	// SDK code. If we can drop support for these and turn them into hard errors,
	// that could let us decouple from the various appengine SDKs. Maybe.
	pvKey     // TODO(riannucci): remove support for this  (use a string)
	pvBlobKey // TODO(riannucci): remove support for this  (use a string)

	pvUNKNOWN
)

func (p *pvals) ReadBinary(buf *bytes.Buffer) error {
	n, _, err := cmpbin.ReadUint(buf)
	if err != nil {
		return err
	}

	p.vals = make([]*typData, n)
	for i := range p.vals {
		p.vals[i] = &typData{}
		err := p.vals[i].ReadBinary(buf, withNS, "")
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *pvals) WriteBinary(buf *bytes.Buffer) error {
	cmpbin.WriteUint(buf, uint64(len(p.vals)))
	for _, v := range p.vals {
		if err := v.WriteBinary(buf, withNS); err != nil {
			return err
		}
	}
	return nil
}
