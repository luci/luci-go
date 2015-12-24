// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/datastore/serialize"
	"github.com/luci/luci-go/common/cmpbin"
)

// TimestampIndexedQueryFields is a log stream QueryBase that contains its
// timestamp in its datastore key.
//
// This grants it a free implicit index and enables sort-by-timestamp queries.
type TimestampIndexedQueryFields struct {
	QueryBase

	// LogStream is the LogStream record key.
	LogStream *ds.Key

	// Timestamp is the timestamp associated with this record.
	Timestamp time.Time
}

var _ interface {
	ds.PropertyLoadSaver
	ds.MetaGetterSetter
} = (*TimestampIndexedQueryFields)(nil)

// Load implements ds.PropertyLoadSaver.
func (s *TimestampIndexedQueryFields) Load(pmap ds.PropertyMap) error {
	if err := s.QueryBase.PLSLoadFrom(pmap); err != nil {
		return err
	}
	return ds.GetPLS(s).Load(pmap)
}

// Save implements ds.PropertyLoadSaver.
func (s *TimestampIndexedQueryFields) Save(withMeta bool) (ds.PropertyMap, error) {
	pmap, err := ds.GetPLS(s).Save(withMeta)
	if err != nil {
		return nil, err
	}
	if err := s.QueryBase.PLSSaveTo(pmap, withMeta); err != nil {
		return nil, err
	}
	return pmap, nil
}

// GetMeta implements ds.MetaGetterSetter.
func (s *TimestampIndexedQueryFields) GetMeta(key string) (interface{}, bool) {
	switch key {
	case "id":
		return s.generateID(), true

	default:
		return ds.GetPLS(s).GetMeta(key)
	}
}

// GetAllMeta implements ds.MetaGetterSetter.
func (s *TimestampIndexedQueryFields) GetAllMeta() ds.PropertyMap {
	pmap := ds.GetPLS(s).GetAllMeta()
	pmap.SetMeta("id", ds.MkProperty(s.generateID()))
	return pmap
}

// SetMeta implements ds.MetaGetterSetter.
func (s *TimestampIndexedQueryFields) SetMeta(key string, val interface{}) bool {
	return ds.GetPLS(s).SetMeta(key, val)
}

// generateID generates the datastore ID from the timestamp entry.
func (s *TimestampIndexedQueryFields) generateID() string {
	pathHash := sha256.New()
	pathHash.Write([]byte(s.Prefix))
	pathHash.Write([]byte(s.Name))

	// Join with '~' rune, which is greater than all of the hex/hash characters.
	return strings.Join([]string{
		generateTimestampID(s.Timestamp),
		hex.EncodeToString(pathHash.Sum(nil)),
	}, "~")
}

func generateTimestampID(t time.Time) string {
	// Encode the time, inverting so that newer times have smaller key values.
	buf := serialize.Invertible(&bytes.Buffer{})
	buf.SetInvert(true)
	cmpbin.WriteInt(buf, NormalizeTime(t).UnixNano())
	return hex.EncodeToString(buf.Bytes())
}

func timestampKey(di ds.Interface, t time.Time) *ds.Key {
	return di.NewKey("TimestampIndexedQueryFields", generateTimestampID(t), 0, nil)
}

// AddNewerFilter adds a filter to the supplied query that restricts it to
// TimestampIndexedQueryFields whose timestamp is on or newer than the supplied
// time.
//
// Because we're storing the inverted timestamp, the newest records will have
// the lowest keys.
func AddNewerFilter(q *ds.Query, di ds.Interface, t time.Time) *ds.Query {
	// Generate our upper bound key, then decrement it (inverted, so reverse
	// order) and use "less than" so we can catch the oldest applicable record.
	//
	// Consider:
	// T0~A
	// T1~B
	//
	// If we generate the timestamp for T1, the key "T1~B" is NOT less than "T1"!
	// If we decrement the search key to "T2" and do a less-than, we will catch
	// any entries beginning with "T1".
	return q.Lt("__key__", timestampKey(di, t.Add(-1*time.Microsecond)))
}

// AddOlderFilter adds a filter to the supplied query that restricts it to
// TimestampIndexedQueryFields whose timestamp is on or older than the supplied
// time.
//
// Because we're storing the inverted timestamp, the oldest records will have
// the highest keys.
func AddOlderFilter(q *ds.Query, di ds.Interface, t time.Time) *ds.Query {
	// Generate our lower bound key. Unlike "Newer", we don't have to modify
	// the generated key since this is a lower bound, all path suffixes will
	// be greater than this key.
	return q.Gt("__key__", timestampKey(di, t))
}
