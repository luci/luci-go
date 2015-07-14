// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package helper

import (
	"encoding/json"
	"infra/gae/libs/gae"
)

// GenericDSKey is an implementation of gae.DSKey which doesn't rely on the
// SDK's implementation. It differs slightly in that it's not recursive (and
// thus cannot express some of the invalid Key's that the SDK implementation
// can). In particular, it's not possible to have a GenericDSKey in a namespace
// whose Parent() is in a different namespace.
//
// GenericDSKey also implements json.Marshaler and json.Unmarshaler so it's
// suitable for use in structs which need to serialize both to json and to
// datastore.
type GenericDSKey struct {
	appID     string
	namespace string
	toks      []gae.DSKeyTok
}

var _ interface {
	gae.DSKey
	json.Marshaler
	json.Unmarshaler
} = (*GenericDSKey)(nil)

// NewDSKeyToks creates a new GenericDSKey. It is the DSKey implementation
// returned from the various DSPropertyMap serialization routines, as well as
// the native key implementation for the in-memory implementation of gae.
func NewDSKeyToks(appID, ns string, toks []gae.DSKeyTok) *GenericDSKey {
	newToks := make([]gae.DSKeyTok, len(toks))
	copy(newToks, toks)
	return &GenericDSKey{appID, ns, newToks}
}

// NewDSKey is a wrapper around NewDSKeyToks which has an interface similar
// to NewKey in the SDK.
func NewDSKey(appID, ns, kind, stringID string, intID int64, parent gae.DSKey) *GenericDSKey {
	_, _, toks := DSKeySplit(parent)
	newToks := make([]gae.DSKeyTok, len(toks))
	copy(newToks, toks)
	newToks = append(newToks, gae.DSKeyTok{Kind: kind, StringID: stringID, IntID: intID})
	return &GenericDSKey{appID, ns, newToks}
}

func (k *GenericDSKey) lastTok() (ret gae.DSKeyTok) {
	if len(k.toks) > 0 {
		ret = k.toks[len(k.toks)-1]
	}
	return
}

func (k *GenericDSKey) AppID() string     { return k.appID }
func (k *GenericDSKey) Namespace() string { return k.namespace }
func (k *GenericDSKey) Kind() string      { return k.lastTok().Kind }
func (k *GenericDSKey) StringID() string  { return k.lastTok().StringID }
func (k *GenericDSKey) IntID() int64      { return k.lastTok().IntID }
func (k *GenericDSKey) String() string    { return DSKeyString(k) }

func (k *GenericDSKey) MarshalJSON() ([]byte, error) {
	return DSKeyMarshalJSON(k)
}

func (k *GenericDSKey) UnmarshalJSON(buf []byte) error {
	appID, namespace, toks, err := DSKeyUnmarshalJSON(buf)
	if err != nil {
		return err
	}
	*k = *NewDSKeyToks(appID, namespace, toks)
	return nil
}

func (k *GenericDSKey) Parent() gae.DSKey {
	if len(k.toks) <= 1 {
		return nil
	}
	return &GenericDSKey{k.appID, k.namespace, k.toks[:len(k.toks)-1]}
}
