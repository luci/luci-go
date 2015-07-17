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

// NewDSKeyFromEncoded decodes and returns a *GenericDSKey
func NewDSKeyFromEncoded(encoded string) (ret *GenericDSKey, err error) {
	ret = &GenericDSKey{}
	ret.appID, ret.namespace, ret.toks, err = DSKeyToksDecode(encoded)
	return
}

func (k *GenericDSKey) lastTok() (ret gae.DSKeyTok) {
	if len(k.toks) > 0 {
		ret = k.toks[len(k.toks)-1]
	}
	return
}

// AppID returns the application ID that this Key is for.
func (k *GenericDSKey) AppID() string { return k.appID }

// Namespace returns the namespace that this Key is for.
func (k *GenericDSKey) Namespace() string { return k.namespace }

// Kind returns the datastore kind of the entity.
func (k *GenericDSKey) Kind() string { return k.lastTok().Kind }

// StringID returns the string ID of the entity (if defined, otherwise "").
func (k *GenericDSKey) StringID() string { return k.lastTok().StringID }

// IntID returns the int64 ID of the entity (if defined, otherwise 0).
func (k *GenericDSKey) IntID() int64 { return k.lastTok().IntID }

// String returns a human-readable version of this Key.
func (k *GenericDSKey) String() string { return DSKeyString(k) }

// Parent returns the parent DSKey of this *GenericDSKey, or nil. The parent
// will always have the concrete type of *GenericDSKey.
func (k *GenericDSKey) Parent() gae.DSKey {
	if len(k.toks) <= 1 {
		return nil
	}
	return &GenericDSKey{k.appID, k.namespace, k.toks[:len(k.toks)-1]}
}

// MarshalJSON allows this key to be automatically marshaled by encoding/json.
func (k *GenericDSKey) MarshalJSON() ([]byte, error) {
	return DSKeyMarshalJSON(k)
}

// UnmarshalJSON allows this key to be automatically unmarshaled by encoding/json.
func (k *GenericDSKey) UnmarshalJSON(buf []byte) error {
	appID, namespace, toks, err := DSKeyUnmarshalJSON(buf)
	if err != nil {
		return err
	}
	*k = *NewDSKeyToks(appID, namespace, toks)
	return nil
}
