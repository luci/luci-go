// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package prod

import (
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/errors"
	"golang.org/x/net/context"
	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
)

type dsKeyImpl struct {
	*datastore.Key
}

// dsR2F (DS real-to-fake) converts an SDK Key to a ds.Key
func dsR2F(k *datastore.Key) *ds.Key {
	if k == nil {
		return nil
	}
	kc := ds.KeyContext{k.AppID(), k.Namespace()}

	count := 0
	for nk := k; nk != nil; nk = nk.Parent() {
		count++
	}

	toks := make([]ds.KeyTok, count)

	for ; k != nil; k = k.Parent() {
		count--
		toks[count].Kind = k.Kind()
		toks[count].StringID = k.StringID()
		toks[count].IntID = k.IntID()
	}
	return kc.NewKeyToks(toks)
}

// dsF2R (DS fake-to-real) converts a DSKey back to an SDK *Key.
func dsF2R(aeCtx context.Context, k *ds.Key) (*datastore.Key, error) {
	if k == nil {
		return nil, nil
	}

	// drop aid.
	_, ns, toks := k.Split()
	err := error(nil)
	aeCtx, err = appengine.Namespace(aeCtx, ns)
	if err != nil {
		return nil, err
	}

	ret := datastore.NewKey(aeCtx, toks[0].Kind, toks[0].StringID, toks[0].IntID, nil)
	for _, t := range toks[1:] {
		ret = datastore.NewKey(aeCtx, t.Kind, t.StringID, t.IntID, ret)
	}

	return ret, nil
}

// dsMF2R (DS multi-fake-to-fake) converts a slice of wrapped keys to SDK keys.
func dsMF2R(aeCtx context.Context, ks []*ds.Key) ([]*datastore.Key, error) {
	lme := errors.NewLazyMultiError(len(ks))
	ret := make([]*datastore.Key, len(ks))
	err := error(nil)
	for i, k := range ks {
		ret[i], err = dsF2R(aeCtx, k)
		lme.Assign(i, err)
	}
	return ret, lme.Get()
}
