// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package datastore

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/golang/protobuf/proto"
	pb "github.com/luci/gae/service/datastore/internal/protos/datastore"
)

// KeyTok is a single token from a multi-part Key.
type KeyTok struct {
	Kind     string
	IntID    int64
	StringID string
}

// Incomplete returns true iff this token doesn't define either a StringID or
// an IntID.
func (k KeyTok) Incomplete() bool {
	return k.StringID == "" && k.IntID == 0
}

// Special returns true iff this token begins and ends with "__"
func (k KeyTok) Special() bool {
	return len(k.Kind) >= 2 && k.Kind[:2] == "__" && k.Kind[len(k.Kind)-2:] == "__"
}

// ID returns the 'active' id as a Property (either the StringID or the IntID).
func (k KeyTok) ID() Property {
	if k.StringID != "" {
		return MkProperty(k.StringID)
	}
	return MkProperty(k.IntID)
}

// Less returns true iff k would sort before other.
func (k KeyTok) Less(other KeyTok) bool {
	if k.Kind < other.Kind {
		return true
	} else if k.Kind > other.Kind {
		return false
	}
	a, b := k.ID(), other.ID()
	return a.Less(&b)
}

// Key is the type used for all datastore operations.
type Key struct {
	appID     string
	namespace string
	toks      []KeyTok
}

var _ interface {
	json.Marshaler
	json.Unmarshaler
} = (*Key)(nil)

// NewKeyToks creates a new Key. It is the Key implementation returned from the
// various PropertyMap serialization routines, as well as the native key
// implementation for the in-memory implementation of gae.
//
// See Interface.NewKeyToks for a version of this function which automatically
// provides aid and ns.
func NewKeyToks(aid, ns string, toks []KeyTok) *Key {
	if len(toks) == 0 {
		return nil
	}
	newToks := make([]KeyTok, len(toks))
	copy(newToks, toks)
	return &Key{aid, ns, newToks}
}

// NewKey is a wrapper around NewToks which has an interface similar
// to NewKey in the SDK.
//
// See Interface.NewKey for a version of this function which automatically
// provides aid and ns.
func NewKey(aid, ns, kind, stringID string, intID int64, parent *Key) *Key {
	if parent == nil {
		return &Key{aid, ns, []KeyTok{{kind, intID, stringID}}}
	}

	toks := parent.toks
	newToks := make([]KeyTok, len(toks), len(toks)+1)
	copy(newToks, toks)
	newToks = append(newToks, KeyTok{kind, intID, stringID})
	return &Key{aid, ns, newToks}
}

// MakeKey is a convenience function for manufacturing a *Key. It should only
// be used when elems... is known statically (e.g. in the code) to be correct.
//
// elems is pairs of (string, string|int|int32|int64) pairs, which correspond to
// Kind/id pairs. Example:
//   MakeKey("aid", "namespace", "Parent", 1, "Child", "id")
//
// Would create the key:
//   aid:namespace:/Parent,1/Child,id
//
// If elems is not parsable (e.g. wrong length, wrong types, etc.) this method
// will panic.
//
// See Interface.MakeKey for a version of this function which automatically
// provides aid and ns.
func MakeKey(aid, ns string, elems ...interface{}) *Key {
	if len(elems) == 0 {
		return nil
	}

	if len(elems)%2 != 0 {
		panic(fmt.Errorf("datastore.MakeKey: odd number of tokens: %v", elems))
	}

	toks := make([]KeyTok, len(elems)/2)
	for i := 0; len(elems) > 0; i, elems = i+1, elems[2:] {
		knd, ok := elems[0].(string)
		if !ok {
			panic(fmt.Errorf("datastore.MakeKey: bad kind: %v", elems[i]))
		}
		t := &toks[i]
		t.Kind = knd
		switch x := elems[1].(type) {
		case string:
			t.StringID = x
		case int:
			t.IntID = int64(x)
		case int32:
			t.IntID = int64(x)
		case int64:
			t.IntID = int64(x)
		default:
			panic(fmt.Errorf("datastore.MakeKey: bad id: %v", x))
		}
	}

	return NewKeyToks(aid, ns, toks)
}

// NewKeyEncoded decodes and returns a *Key
func NewKeyEncoded(encoded string) (ret *Key, err error) {
	ret = &Key{}
	// Re-add padding
	if m := len(encoded) % 4; m != 0 {
		encoded += strings.Repeat("=", 4-m)
	}
	b, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return
	}

	r := &pb.Reference{}
	if err = proto.Unmarshal(b, r); err != nil {
		return
	}

	ret.appID = r.GetApp()
	ret.namespace = r.GetNameSpace()
	ret.toks = make([]KeyTok, len(r.Path.Element))
	for i, e := range r.Path.Element {
		ret.toks[i] = KeyTok{
			Kind:     e.GetType(),
			IntID:    e.GetId(),
			StringID: e.GetName(),
		}
	}
	return
}

// LastTok returns the last KeyTok in this Key. Non-nil Keys are always guaranteed
// to have at least one token.
func (k *Key) LastTok() KeyTok {
	return k.toks[len(k.toks)-1]
}

// AppID returns the application ID that this Key is for.
func (k *Key) AppID() string { return k.appID }

// Namespace returns the namespace that this Key is for.
func (k *Key) Namespace() string { return k.namespace }

// Kind returns the Kind of the child KeyTok
func (k *Key) Kind() string { return k.toks[len(k.toks)-1].Kind }

// StringID returns the StringID of the child KeyTok
func (k *Key) StringID() string { return k.toks[len(k.toks)-1].StringID }

// IntID returns the IntID of the child KeyTok
func (k *Key) IntID() int64 { return k.toks[len(k.toks)-1].IntID }

// String returns a human-readable representation of the key in the form of
//   AID:NS:/Kind,id/Kind,id/...
func (k *Key) String() string {
	b := bytes.NewBuffer(make([]byte, 0, 512))
	fmt.Fprintf(b, "%s:%s:", k.appID, k.namespace)
	for _, t := range k.toks {
		if t.StringID != "" {
			fmt.Fprintf(b, "/%s,%q", t.Kind, t.StringID)
		} else {
			fmt.Fprintf(b, "/%s,%d", t.Kind, t.IntID)
		}
	}
	return b.String()
}

// Incomplete returns true iff k doesn't have an id yet.
func (k *Key) Incomplete() bool {
	return k.LastTok().Incomplete()
}

// Valid determines if a key is valid, according to a couple rules:
//   - k is not nil
//   - every token of k:
//     - (if !allowSpecial) token's kind doesn't start with '__'
//     - token's kind and appid are non-blank
//     - token is not incomplete
//   - all tokens have the same namespace and appid
func (k *Key) Valid(allowSpecial bool, aid, ns string) bool {
	if aid != k.appID || ns != k.namespace {
		return false
	}
	for _, t := range k.toks {
		if t.Incomplete() {
			return false
		}
		if !allowSpecial && t.Special() {
			return false
		}
		if t.Kind == "" {
			return false
		}
		if t.StringID != "" && t.IntID != 0 {
			return false
		}
	}
	return true
}

// PartialValid returns true iff this key is suitable for use in a Put
// operation. This is the same as Valid(k, false, ...), but also allowing k to
// be Incomplete().
func (k *Key) PartialValid(aid, ns string) bool {
	if k.Incomplete() {
		k = NewKey(k.AppID(), k.Namespace(), k.Kind(), "", 1, k.Parent())
	}
	return k.Valid(false, aid, ns)
}

// Parent returns the parent Key of this *Key, or nil. The parent
// will always have the concrete type of *Key.
func (k *Key) Parent() *Key {
	if len(k.toks) <= 1 {
		return nil
	}
	return &Key{k.appID, k.namespace, k.toks[:len(k.toks)-1]}
}

// MarshalJSON allows this key to be automatically marshaled by encoding/json.
func (k *Key) MarshalJSON() ([]byte, error) {
	return []byte(`"` + k.Encode() + `"`), nil
}

// Encode encodes the provided key as a base64-encoded protobuf.
//
// This encoding is compatible with the SDK-provided encoding and is agnostic
// to the underlying implementation of the Key.
//
// It's encoded with the urlsafe base64 table without padding.
func (k *Key) Encode() string {
	e := make([]*pb.Path_Element, len(k.toks))
	for i, t := range k.toks {
		t := t
		e[i] = &pb.Path_Element{
			Type: &t.Kind,
		}
		if t.StringID != "" {
			e[i].Name = &t.StringID
		} else {
			e[i].Id = &t.IntID
		}
	}
	var namespace *string
	if k.namespace != "" {
		namespace = &k.namespace
	}
	r, err := proto.Marshal(&pb.Reference{
		App:       &k.appID,
		NameSpace: namespace,
		Path: &pb.Path{
			Element: e,
		},
	})
	if err != nil {
		panic(err)
	}

	// trim padding
	return strings.TrimRight(base64.URLEncoding.EncodeToString(r), "=")
}

// UnmarshalJSON allows this key to be automatically unmarshaled by encoding/json.
func (k *Key) UnmarshalJSON(buf []byte) error {
	if len(buf) < 2 || buf[0] != '"' || buf[len(buf)-1] != '"' {
		return errors.New("datastore: bad JSON key")
	}
	nk, err := NewKeyEncoded(string(buf[1 : len(buf)-1]))
	if err != nil {
		return err
	}
	*k = *nk
	return nil
}

// GobEncode allows the Key to be encoded in a Gob struct.
func (k *Key) GobEncode() ([]byte, error) {
	return []byte(k.Encode()), nil
}

// GobDecode allows the Key to be decoded in a Gob struct.
func (k *Key) GobDecode(buf []byte) error {
	nk, err := NewKeyEncoded(string(buf))
	if err != nil {
		return err
	}
	*k = *nk
	return nil
}

// Root returns the entity root for the given key.
func (k *Key) Root() *Key {
	if len(k.toks) > 1 {
		ret := *k
		ret.toks = ret.toks[:1]
		return &ret
	}
	return k
}

// Less returns true iff k would sort before other.
func (k *Key) Less(other *Key) bool {
	if k.appID < other.appID {
		return true
	} else if k.appID > other.appID {
		return false
	}

	if k.namespace < other.namespace {
		return true
	} else if k.namespace > other.namespace {
		return false
	}

	lim := len(k.toks)
	if len(other.toks) < lim {
		lim = len(other.toks)
	}
	for i := 0; i < lim; i++ {
		a, b := k.toks[i], other.toks[i]
		if a.Less(b) {
			return true
		} else if b.Less(a) {
			return false
		}
	}
	return len(k.toks) < len(other.toks)
}

// HasAncestor returns true iff other is an ancestor of k (or if other == k).
func (k *Key) HasAncestor(other *Key) bool {
	if k.appID != other.appID || k.namespace != other.namespace {
		return false
	}
	if len(k.toks) < len(other.toks) {
		return false
	}
	for i, tok := range other.toks {
		if tok != k.toks[i] {
			return false
		}
	}
	return true
}

// GQL returns a correctly formatted Cloud Datastore GQL key literal.
//
// The flavor of GQL that this emits is defined here:
//   https://cloud.google.com/datastore/docs/apis/gql/gql_reference
func (k *Key) GQL() string {
	ret := &bytes.Buffer{}
	fmt.Fprintf(ret, "KEY(DATASET(%s)", gqlQuoteString(k.appID))
	if k.namespace != "" {
		fmt.Fprintf(ret, ", NAMESPACE(%s)", gqlQuoteString(k.namespace))
	}
	for _, t := range k.toks {
		if t.IntID != 0 {
			fmt.Fprintf(ret, ", %s, %d", gqlQuoteString(t.Kind), t.IntID)
		} else {
			fmt.Fprintf(ret, ", %s, %s", gqlQuoteString(t.Kind), gqlQuoteString(t.StringID))
		}
	}
	if _, err := ret.WriteString(")"); err != nil {
		panic(err)
	}
	return ret.String()
}

// Equal returns true iff the two keys represent identical key values.
func (k *Key) Equal(other *Key) (ret bool) {
	ret = (k.appID == other.appID &&
		k.namespace == other.namespace &&
		len(k.toks) == len(other.toks))
	if ret {
		for i, t := range k.toks {
			if ret = t == other.toks[i]; !ret {
				return
			}
		}
	}
	return
}

// Split componentizes the key into pieces (AppID, Namespace and tokens)
//
// Each token represents one piece of they key's 'path'.
//
// toks is guaranteed to be empty if and only if k is nil. If k is non-nil then
// it contains at least one token.
func (k *Key) Split() (appID, namespace string, toks []KeyTok) {
	appID = k.appID
	namespace = k.namespace
	toks = make([]KeyTok, len(k.toks))
	copy(toks, k.toks)
	return
}

// EstimateSize estimates the size of a Key.
//
// It uses https://cloud.google.com/appengine/articles/storage_breakdown?csw=1
// as a guide for these values.
func (k *Key) EstimateSize() int64 {
	ret := int64(len(k.appID))
	ret += int64(len(k.namespace))
	for _, t := range k.toks {
		ret += int64(len(t.Kind))
		if t.StringID != "" {
			ret += int64(len(t.StringID))
		} else {
			ret += 8
		}
	}
	return ret
}
