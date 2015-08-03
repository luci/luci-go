// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package datastore

import (
	"encoding/json"
)

// GenericKey is an implementation of Key which doesn't rely on the
// SDK's implementation. It differs slightly in that it's not recursive (and
// thus cannot express some of the invalid Key's that the SDK implementation
// can). In particular, it's not possible to have a GenericKey in a namespace
// whose Parent() is in a different namespace.
//
// GenericKey also implements json.Marshaler and json.Unmarshaler so it's
// suitable for use in structs which need to serialize both to json and to
// datastore.
type GenericKey struct {
	appID     string
	namespace string
	toks      []KeyTok
}

var _ interface {
	Key
	json.Marshaler
	json.Unmarshaler
} = (*GenericKey)(nil)

// NewKeyToks creates a new GenericKey. It is the Key implementation
// returned from the various PropertyMap serialization routines, as well as
// the native key implementation for the in-memory implementation of gae.
func NewKeyToks(appID, ns string, toks []KeyTok) *GenericKey {
	newToks := make([]KeyTok, len(toks))
	copy(newToks, toks)
	return &GenericKey{appID, ns, newToks}
}

// NewKey is a wrapper around NewKeyToks which has an interface similar
// to NewKey in the SDK.
func NewKey(appID, ns, kind, stringID string, intID int64, parent Key) *GenericKey {
	_, _, toks := KeySplit(parent)
	newToks := make([]KeyTok, len(toks))
	copy(newToks, toks)
	newToks = append(newToks, KeyTok{Kind: kind, StringID: stringID, IntID: intID})
	return &GenericKey{appID, ns, newToks}
}

// NewKeyFromEncoded decodes and returns a *GenericKey
func NewKeyFromEncoded(encoded string) (ret *GenericKey, err error) {
	ret = &GenericKey{}
	ret.appID, ret.namespace, ret.toks, err = KeyToksDecode(encoded)
	return
}

func (k *GenericKey) lastTok() (ret KeyTok) {
	if len(k.toks) > 0 {
		ret = k.toks[len(k.toks)-1]
	}
	return
}

// AppID returns the application ID that this Key is for.
func (k *GenericKey) AppID() string { return k.appID }

// Namespace returns the namespace that this Key is for.
func (k *GenericKey) Namespace() string { return k.namespace }

// Kind returns the datastore kind of the entity.
func (k *GenericKey) Kind() string { return k.lastTok().Kind }

// StringID returns the string ID of the entity (if defined, otherwise "").
func (k *GenericKey) StringID() string { return k.lastTok().StringID }

// IntID returns the int64 ID of the entity (if defined, otherwise 0).
func (k *GenericKey) IntID() int64 { return k.lastTok().IntID }

// String returns a human-readable version of this Key.
func (k *GenericKey) String() string { return KeyString(k) }

// Parent returns the parent Key of this *GenericKey, or nil. The parent
// will always have the concrete type of *GenericKey.
func (k *GenericKey) Parent() Key {
	if len(k.toks) <= 1 {
		return nil
	}
	return &GenericKey{k.appID, k.namespace, k.toks[:len(k.toks)-1]}
}

// MarshalJSON allows this key to be automatically marshaled by encoding/json.
func (k *GenericKey) MarshalJSON() ([]byte, error) {
	return KeyMarshalJSON(k)
}

// UnmarshalJSON allows this key to be automatically unmarshaled by encoding/json.
func (k *GenericKey) UnmarshalJSON(buf []byte) error {
	appID, namespace, toks, err := KeyUnmarshalJSON(buf)
	if err != nil {
		return err
	}
	*k = *NewKeyToks(appID, namespace, toks)
	return nil
}
