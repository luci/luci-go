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
	"errors"
	"fmt"
	"time"

	"go.chromium.org/luci/common/data/cmpbin"

	"go.chromium.org/luci/gae/service/blobstore"
)

const (
	// readPropertyMapReasonableLimit sets a limit on the number of rows and
	// number of properties per row which can be read by ReadPropertyMap. The
	// total number of Property objects readable by this method is this number
	// squared (e.g. Limit rows * Limit properties)
	readPropertyMapReasonableLimit uint64 = 30000

	// maxIndexColumns is the maximum number of sort columns (e.g. sort orders)
	// that ReadIndexDefinition is willing to deserialize. 64 was chosen as
	// a likely-astronomical number.
	maxIndexColumns = 64

	// readKeyNumToksReasonableLimit is the maximum number of Key tokens that
	// ReadKey is willing to read for a single key.
	readKeyNumToksReasonableLimit = 50
)

// Deserializer allows reading binary-encoded datastore types (like Properties,
// Keys, etc.)
//
// See the `Deserialize` variable for a common shortcut.
type Deserializer struct {
	// If empty, this Deserializer will use the appid and namespace encoded in
	// *Key objects (if any).
	//
	// If supplied, any encoded appid/namespace will be ignored and this will be
	// used to fill in the appid and namespace for all returned *Key objects.
	KeyContext KeyContext
}

// Deserialize is a Deserializer without KeyContext (i.e. appid/namespace
// encoded in Keys will be returned). Useful for inline invocations like:
//
//	datastore.Deserialize.Time(...)
var Deserialize Deserializer

// Key deserializes a key from the buffer. The value of context must match
// the value of context that was passed to WriteKey when the key was encoded.
// If context == WithoutContext, then the appid and namespace parameters are
// used in the decoded Key. Otherwise they're ignored.
func (d Deserializer) Key(buf cmpbin.ReadableBytesBuffer) (ret *Key, err error) {
	defer recoverTo(&err)
	actualCtx, e := buf.ReadByte()
	panicIf(e)

	var kc KeyContext
	if actualCtx == 1 {
		kc.AppID, _, e = cmpbin.ReadString(buf)
		panicIf(e)
		kc.Namespace, _, e = cmpbin.ReadString(buf)
		panicIf(e)
	} else if actualCtx != 0 {
		err = fmt.Errorf("helper: expected actualCtx to be 0 or 1, got %d", actualCtx)
		return
	}

	if d.KeyContext.AppID != "" || d.KeyContext.Namespace != "" {
		// overrwrite with the supplied ones
		kc = d.KeyContext
	}

	toks := []KeyTok{}
	for {
		ctrlByte, e := buf.ReadByte()
		panicIf(e)
		if ctrlByte == 0 {
			break
		}
		if len(toks)+1 > readKeyNumToksReasonableLimit {
			err = fmt.Errorf(
				"helper: tried to decode huge key with > %d tokens",
				readKeyNumToksReasonableLimit)
			return
		}

		tok, e := d.KeyTok(buf)
		panicIf(e)

		toks = append(toks, tok)
	}

	return kc.NewKeyToks(toks), nil
}

// KeyTok reads a KeyTok from the buffer. You usually want ReadKey
// instead of this.
func (d Deserializer) KeyTok(buf cmpbin.ReadableBytesBuffer) (ret KeyTok, err error) {
	defer recoverTo(&err)
	e := error(nil)
	ret.Kind, _, e = cmpbin.ReadString(buf)
	panicIf(e)

	typ, e := buf.ReadByte()
	panicIf(e)

	switch PropertyType(typ) {
	case PTString:
		ret.StringID, _, err = cmpbin.ReadString(buf)
	case PTInt:
		ret.IntID, _, err = cmpbin.ReadInt(buf)
		if err == nil && ret.IntID <= 0 {
			err = errors.New("helper: decoded key with empty stringID and zero/negative intID")
		}
	default:
		err = fmt.Errorf("helper: invalid type %s", PropertyType(typ))
	}
	return
}

// GeoPoint reads a GeoPoint from the buffer.
func (d Deserializer) GeoPoint(buf cmpbin.ReadableBytesBuffer) (gp GeoPoint, err error) {
	defer recoverTo(&err)
	e := error(nil)
	gp.Lat, _, e = cmpbin.ReadFloat64(buf)
	panicIf(e)

	gp.Lng, _, e = cmpbin.ReadFloat64(buf)
	panicIf(e)

	if !gp.Valid() {
		err = fmt.Errorf("helper: decoded invalid GeoPoint: %v", gp)
	}
	return
}

// Time reads a time.Time from the buffer.
func (d Deserializer) Time(buf cmpbin.ReadableBytesBuffer) (time.Time, error) {
	v, _, err := cmpbin.ReadInt(buf)
	if err != nil {
		return time.Time{}, err
	}
	return IntToTime(v), nil
}

// Property reads a Property from the buffer. `context` and `kc` behave the
// same way they do for Key, but only have an effect if the decoded property
// has a Key value.
func (d Deserializer) Property(buf cmpbin.ReadableBytesBuffer) (p Property, err error) {
	val := any(nil)
	b, err := buf.ReadByte()
	if err != nil {
		return
	}
	is := ShouldIndex
	if (b & 0x80) == 0 {
		is = NoIndex
	}
	switch PropertyType(b & 0x7f) {
	case PTNull:
	case PTBool:
		b, err = buf.ReadByte()
		val = (b != 0)
	case PTInt:
		val, _, err = cmpbin.ReadInt(buf)
	case PTFloat:
		val, _, err = cmpbin.ReadFloat64(buf)
	case PTString:
		val, _, err = cmpbin.ReadString(buf)
	case PTBytes:
		val, _, err = cmpbin.ReadBytes(buf)
	case PTTime:
		val, err = d.Time(buf)
	case PTGeoPoint:
		val, err = d.GeoPoint(buf)
	case PTPropertyMap:
		val, err = d.propertyMap(buf, true)
	case PTKey:
		val, err = d.Key(buf)
	case PTBlobKey:
		s := ""
		if s, _, err = cmpbin.ReadString(buf); err != nil {
			break
		}
		val = blobstore.Key(s)
	default:
		err = fmt.Errorf("read: unknown type! %v", b)
	}
	if err == nil {
		err = p.SetValue(val, is)
	}
	return
}

// PropertyMap reads a top-level PropertyMap from the buffer. `context` and
// friends behave the same way that they do for ReadKey.
func (d Deserializer) PropertyMap(buf cmpbin.ReadableBytesBuffer) (PropertyMap, error) {
	return d.propertyMap(buf, false)
}

// PropertyMap reads a PropertyMap from the buffer. `context` and
// friends behave the same way that they do for ReadKey.
//
// If `populateKey` is true, will recognize meta properties representing a key
// (skipping any other meta property) and convert them into the same format as
// produced by PopulateKey. That way property map consumers can pick properties
// they care about. This is used when loading nested property maps. The key of
// a top-level property map is propagated using another mechanism.
//
// If `populateKey` is false meta properties are ignored.
func (d Deserializer) propertyMap(buf cmpbin.ReadableBytesBuffer, populateKey bool) (pm PropertyMap, err error) {
	defer recoverTo(&err)

	numRows := uint64(0)
	numRows, _, e := cmpbin.ReadUint(buf)
	panicIf(e)
	if numRows > readPropertyMapReasonableLimit {
		err = fmt.Errorf("helper: tried to decode map with huge number of rows %d", numRows)
		return
	}

	capacity := numRows
	if populateKey {
		capacity += 3 // extra for storing `$id`, `$kind` and `$parent`
	}

	var meta PropertyMap
	pm = make(PropertyMap, capacity)

	name, prop := "", Property{}
	for i := uint64(0); i < numRows; i++ {
		name, _, e = cmpbin.ReadString(buf)
		panicIf(e)

		dest := pm
		if isMetaKey(name) {
			if meta == nil {
				meta = make(PropertyMap, 1)
			}
			dest = meta
		}

		numProps, _, e := cmpbin.ReadInt(buf)
		panicIf(e)
		switch {
		case numProps < 0:
			// Single property.
			prop, err = d.Property(buf)
			panicIf(err)
			dest[name] = prop

		case uint64(numProps) > readPropertyMapReasonableLimit:
			err = fmt.Errorf("helper: tried to decode map with huge number of properties %d", numProps)
			return

		default:
			props := make(PropertySlice, 0, numProps)
			for j := int64(0); j < numProps; j++ {
				prop, err = d.Property(buf)
				panicIf(err)
				props = append(props, prop)
			}
			dest[name] = props
		}
	}

	// This basically "normalizes" how a key is represented in meta fields, e.g.
	// a single `$key` is expanded into `$id`, `$kind`, etc. We don't know how
	// the caller wants their meta properties populated, so need to set them all.
	if populateKey {
		if key, _ := d.KeyContext.NewKeyFromMeta(meta); key != nil {
			PopulateKey(pm, key)
		}
	}

	return
}

// IndexColumn reads an IndexColumn from the buffer.
func (d Deserializer) IndexColumn(buf cmpbin.ReadableBytesBuffer) (c IndexColumn, err error) {
	defer recoverTo(&err)

	dir, err := buf.ReadByte()
	panicIf(err)

	c.Descending = dir != 0
	c.Property, _, err = cmpbin.ReadString(buf)
	return
}

// IndexDefinition reads an IndexDefinition from the buffer.
func (d Deserializer) IndexDefinition(buf cmpbin.ReadableBytesBuffer) (i IndexDefinition, err error) {
	defer recoverTo(&err)

	i.Kind, _, err = cmpbin.ReadString(buf)
	panicIf(err)

	anc, err := buf.ReadByte()
	panicIf(err)

	i.Ancestor = anc == 1

	for {
		ctrl := byte(0)
		ctrl, err = buf.ReadByte()
		panicIf(err)
		if ctrl == 0 {
			break
		}
		if len(i.SortBy) > maxIndexColumns {
			err = fmt.Errorf("datastore: Got over %d sort orders", maxIndexColumns)
			return
		}

		sb, err := d.IndexColumn(buf)
		panicIf(err)

		i.SortBy = append(i.SortBy, sb)
	}

	return
}
