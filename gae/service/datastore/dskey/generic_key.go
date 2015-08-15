// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dskey

import (
	"encoding/json"

	ds "github.com/luci/gae/service/datastore"
)

// Generic is an implementation of Key which doesn't rely on the
// SDK's implementation. It differs slightly in that it's not recursive (and
// thus cannot express some of the invalid Key's that the SDK implementation
// can). In particular, it's not possible to have a Generic in a namespace
// whose Parent() is in a different namespace.
//
// Generic also implements json.Marshaler and json.Unmarshaler so it's
// suitable for use in structs which need to serialize both to json and to
// datastore.
type Generic struct {
	appID     string
	namespace string
	toks      []ds.KeyTok
}

var _ interface {
	ds.Key
	json.Marshaler
	json.Unmarshaler
} = (*Generic)(nil)

// NewToks creates a new Generic. It is the Key implementation
// returned from the various PropertyMap serialization routines, as well as
// the native key implementation for the in-memory implementation of gae.
func NewToks(appID, ns string, toks []ds.KeyTok) *Generic {
	newToks := make([]ds.KeyTok, len(toks))
	copy(newToks, toks)
	return &Generic{appID, ns, newToks}
}

// New is a wrapper around NewToks which has an interface similar
// to NewKey in the SDK.
func New(appID, ns, kind, stringID string, intID int64, parent ds.Key) *Generic {
	_, _, toks := Split(parent)
	newToks := make([]ds.KeyTok, len(toks))
	copy(newToks, toks)
	newToks = append(newToks, ds.KeyTok{Kind: kind, StringID: stringID, IntID: intID})
	return &Generic{appID, ns, newToks}
}

// NewFromEncoded decodes and returns a *Generic
func NewFromEncoded(encoded string) (ret *Generic, err error) {
	ret = &Generic{}
	ret.appID, ret.namespace, ret.toks, err = ToksDecode(encoded)
	return
}

func (k *Generic) lastTok() ds.KeyTok {
	if len(k.toks) > 0 {
		return k.toks[len(k.toks)-1]
	}
	return ds.KeyTok{}
}

// AppID returns the application ID that this Key is for.
func (k *Generic) AppID() string { return k.appID }

// Namespace returns the namespace that this Key is for.
func (k *Generic) Namespace() string { return k.namespace }

// Kind returns the datastore kind of the entity.
func (k *Generic) Kind() string { return k.lastTok().Kind }

// StringID returns the string ID of the entity (if defined, otherwise "").
func (k *Generic) StringID() string { return k.lastTok().StringID }

// IntID returns the int64 ID of the entity (if defined, otherwise 0).
func (k *Generic) IntID() int64 { return k.lastTok().IntID }

// String returns a human-readable version of this Key.
func (k *Generic) String() string { return String(k) }

// Incomplete returns true iff k doesn't have an id yet
func (k *Generic) Incomplete() bool { return Incomplete(k) }

// Valid returns true if this key is complete and suitable for use with the
// datastore. See the package-level Valid function for more information.
func (k *Generic) Valid(allowSpecial bool, aid, ns string) bool {
	return Valid(k, allowSpecial, aid, ns)
}

// PartialValid returns true if this key is suitable for use with a Put
// operation. See the package-level PartialValid function for more information.
func (k *Generic) PartialValid(aid, ns string) bool {
	return PartialValid(k, aid, ns)
}

// Parent returns the parent Key of this *Generic, or nil. The parent
// will always have the concrete type of *Generic.
func (k *Generic) Parent() ds.Key {
	if len(k.toks) <= 1 {
		return nil
	}
	return &Generic{k.appID, k.namespace, k.toks[:len(k.toks)-1]}
}

// MarshalJSON allows this key to be automatically marshaled by encoding/json.
func (k *Generic) MarshalJSON() ([]byte, error) {
	return MarshalJSON(k)
}

// UnmarshalJSON allows this key to be automatically unmarshaled by encoding/json.
func (k *Generic) UnmarshalJSON(buf []byte) error {
	appID, namespace, toks, err := UnmarshalJSON(buf)
	if err != nil {
		return err
	}
	*k = *NewToks(appID, namespace, toks)
	return nil
}
