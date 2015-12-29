// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tumble

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"math"
	"reflect"
	"sync"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/appengine/meta"
	"github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

var registry = map[string]reflect.Type{}

// Register allows |mut| to be played by the tumble backend. This should be
// called at init() time once for every Mutation implementation.
//
// This will also gob.Register your mutation implementation.
//
// Example:
//   Register((*MyMutationImpl)(nil))
func Register(mut Mutation) {
	gob.Register(mut)
	t := reflect.TypeOf(mut)
	registry[t.String()] = t
}

// Mutation is the interface that your tumble mutations must implement.
//
// Mutation implementations can be registered with the Register function.
type Mutation interface {
	// Root returns a datastore.Key which will be used to derive the Key for the
	// entity group which this Mutation will operate on. This is used to batch
	// together Entries for more efficient processing.
	//
	// Returning nil is an error.
	Root(c context.Context) *datastore.Key

	// RollForward performs the action of the Mutation.
	//
	// It is only considered sucessful if it returns nil. If it returns non-nil,
	// then it will be retried at a later time. If it never returns nil, then it
	// will never be flushed from tumble's queue, and you'll have to manually
	// delete it or fix the code so that it can be handled without error.
	//
	// This method runs inside of a single-group transaction. It must modify only
	// the entity group specified by Root().
	//
	// As a side effect, RollForward may return new arbitrary Mutations. These
	// will be comitted in the same transaction as RollForward.
	//
	// The context contains an implementation of "luci/gae/service/datastore",
	// using the "luci/gae/filter/txnBuf" transaction buffer. This means that
	// all functionality (including additional transactions) is available, with
	// the limitations mentioned by that package (notably, no cursors are
	// allowed).
	RollForward(c context.Context) ([]Mutation, error)
}

type realMutation struct {
	// TODO(riannucci): add functionality to luci/gae/service/datastore so that
	// GetMeta/SetMeta may be overridden by the struct.
	_kind  string         `gae:"$kind,tumble.Mutation"`
	ID     string         `gae:"$id"`
	Parent *datastore.Key `gae:"$parent"`

	ExpandedShard int64
	TargetRoot    *datastore.Key

	Version string
	Type    string
	Data    []byte `gae:",noindex"`
}

func (r *realMutation) shard(cfg *Config) uint64 {
	expandedShardsPerShard := math.MaxUint64 / cfg.NumShards
	ret := uint64(r.ExpandedShard-math.MinInt64) / expandedShardsPerShard
	// account for rounding errors on the last shard.
	if ret >= cfg.NumShards {
		ret = cfg.NumShards - 1
	}
	return ret
}

func putMutations(c context.Context, fromRoot *datastore.Key, muts []Mutation, round uint64) (shardSet map[uint64]struct{}, mutKeys []*datastore.Key, err error) {
	cfg := GetConfig(c)

	if len(muts) == 0 {
		return
	}

	version, err := meta.GetEntityGroupVersion(c, fromRoot)
	if err != nil {
		return
	}

	ds := datastore.Get(c)

	shardSet = map[uint64]struct{}{}
	toPut := make([]*realMutation, len(muts))
	mutKeys = make([]*datastore.Key, len(muts))
	for i, m := range muts {
		id := fmt.Sprintf("%016x_%08x_%08x", version, round, i)
		toPut[i], err = newRealMutation(c, id, fromRoot, m)
		if err != nil {
			logging.Errorf(c, "error creating real mutation for %v: %s", m, err)
			return
		}
		mutKeys[i] = ds.KeyForObj(toPut[i])

		shardSet[toPut[i].shard(&cfg)] = struct{}{}
	}

	if err = ds.PutMulti(toPut); err != nil {
		logging.Errorf(c, "error putting %d new mutations: %s", len(toPut), err)
	}
	return
}

var appVersion = struct {
	sync.Once
	version string
}{}

func getAppVersion(c context.Context) string {
	appVersion.Do(func() {
		appVersion.version = info.Get(c).VersionID()
	})
	return appVersion.version
}

func newRealMutation(c context.Context, id string, parent *datastore.Key, m Mutation) (*realMutation, error) {
	t := reflect.TypeOf(m).String()
	if _, ok := registry[t]; !ok {
		return nil, fmt.Errorf("Attempting to add unregistered mutation %v: %v", t, m)
	}

	buf := &bytes.Buffer{}
	err := gob.NewEncoder(buf).Encode(m)
	if err != nil {
		return nil, err
	}

	root := m.Root(c).Root()

	hash := sha1.Sum([]byte(root.Encode()))
	eshard := int64(binary.BigEndian.Uint64(hash[:]))

	return &realMutation{
		ID:     id,
		Parent: parent,

		ExpandedShard: eshard,
		TargetRoot:    root,

		Version: getAppVersion(c),
		Type:    t,
		Data:    buf.Bytes(),
	}, nil
}

func (r *realMutation) GetMutation() (Mutation, error) {
	typ, ok := registry[r.Type]
	if !ok {
		return nil, fmt.Errorf("unable to load reflect.Type for %q", r.Type)
	}

	ret := reflect.New(typ)
	if err := gob.NewDecoder(bytes.NewBuffer(r.Data)).DecodeValue(ret); err != nil {
		return nil, err
	}

	return ret.Elem().Interface().(Mutation), nil
}
