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
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/golang/protobuf/proto"
	pb "go.chromium.org/gae/service/datastore/internal/protos/datastore"
)

// KeyTok is a single token from a multi-part Key.
type KeyTok struct {
	Kind     string
	IntID    int64
	StringID string
}

// IsIncomplete returns true iff this token doesn't define either a StringID or
// an IntID.
func (k KeyTok) IsIncomplete() bool {
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

// KeyContext is the context in which a key is generated.
type KeyContext struct {
	AppID     string
	Namespace string
}

// MkKeyContext is a helper function to create a new KeyContext.
//
// It is preferable to field-based struct initialization because, as a function,
// it has the ability to enforce an exact number of parameters.
func MkKeyContext(appID, namespace string) KeyContext {
	return KeyContext{AppID: appID, Namespace: namespace}
}

// Matches returns true iff the AppID and Namespace parameters are the same for
// the two KeyContext instances.
func (kc KeyContext) Matches(o KeyContext) bool {
	return (kc.AppID == o.AppID && kc.Namespace == o.Namespace)
}

// NewKeyToks creates a new Key. It is the Key implementation returned from
// the various PropertyMap serialization routines, as well as the native key
// implementation for the in-memory implementation of gae.
//
// See NewKeyToks for a version of this function which automatically
// provides aid and ns.
func (kc KeyContext) NewKeyToks(toks []KeyTok) *Key {
	if len(toks) == 0 {
		return nil
	}
	newToks := make([]KeyTok, len(toks))
	copy(newToks, toks)
	return &Key{kc, newToks}
}

// NewKey is a wrapper around NewToks which has an interface similar to NewKey
// in the SDK.
//
// See NewKey for a version of this function which automatically provides aid
// and ns.
func (kc KeyContext) NewKey(kind, stringID string, intID int64, parent *Key) *Key {
	if parent == nil {
		return &Key{kc, []KeyTok{{kind, intID, stringID}}}
	}

	toks := parent.toks
	newToks := make([]KeyTok, len(toks), len(toks)+1)
	copy(newToks, toks)
	newToks = append(newToks, KeyTok{kind, intID, stringID})
	return &Key{kc, newToks}
}

// MakeKey is a convenience function for manufacturing a *Key. It should only
// be used when elems... is known statically (e.g. in the code) to be correct.
//
// elems is pairs of (string, string|int|int32|int64) pairs, which correspond to
// Kind/id pairs. Example:
//   MkKeyContext("aid", "namespace").MakeKey("Parent", 1, "Child", "id")
//
// Would create the key:
//   aid:namespace:/Parent,1/Child,id
//
// If elems is not parsable (e.g. wrong length, wrong types, etc.) this method
// will panic.
//
// See MakeKey for a version of this function which automatically
// provides aid and ns.
func (kc KeyContext) MakeKey(elems ...interface{}) *Key {
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
		case uint16:
			t.IntID = int64(x)
		case uint32:
			t.IntID = int64(x)
		default:
			panic(fmt.Errorf("datastore.MakeKey: bad id: %v", x))
		}
	}

	return kc.NewKeyToks(toks)
}

// Key is the type used for all datastore operations.
type Key struct {
	kc   KeyContext
	toks []KeyTok
}

var _ interface {
	json.Marshaler
	json.Unmarshaler
} = (*Key)(nil)

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

	ret.kc = MkKeyContext(r.GetApp(), r.GetNameSpace())
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
func (k *Key) AppID() string { return k.kc.AppID }

// Namespace returns the namespace that this Key is for.
func (k *Key) Namespace() string { return k.kc.Namespace }

// KeyContext returns the KeyContext that this Key is using.
func (k *Key) KeyContext() *KeyContext {
	kc := k.kc
	return &kc
}

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
	fmt.Fprintf(b, "%s:%s:", k.kc.AppID, k.kc.Namespace)
	for _, t := range k.toks {
		if t.StringID != "" {
			fmt.Fprintf(b, "/%s,%q", t.Kind, t.StringID)
		} else {
			fmt.Fprintf(b, "/%s,%d", t.Kind, t.IntID)
		}
	}
	return b.String()
}

// IsIncomplete returns true iff the last token of this Key doesn't define
// either a StringID or an IntID.
func (k *Key) IsIncomplete() bool {
	return k.LastTok().IsIncomplete()
}

// Valid determines if a key is valid, according to a couple of rules:
//   - k is not nil
//   - every token of k:
//     - (if !allowSpecial) token's kind doesn't start with '__'
//     - token's kind and appid are non-blank
//     - token is not incomplete
//   - all tokens have the same namespace and appid
func (k *Key) Valid(allowSpecial bool, kc KeyContext) bool {
	if !kc.Matches(k.kc) {
		return false
	}
	for _, t := range k.toks {
		if t.IsIncomplete() {
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
// be IsIncomplete().
func (k *Key) PartialValid(kc KeyContext) bool {
	if k.IsIncomplete() {
		if !kc.Matches(k.kc) {
			return false
		}
		k = kc.NewKey(k.Kind(), "", 1, k.Parent())
	}
	return k.Valid(false, kc)
}

// Parent returns the parent Key of this *Key, or nil. The parent
// will always have the concrete type of *Key.
func (k *Key) Parent() *Key {
	if len(k.toks) <= 1 {
		return nil
	}
	return k.kc.NewKeyToks(k.toks[:len(k.toks)-1])
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
	if ns := k.kc.Namespace; ns != "" {
		namespace = &ns
	}
	r, err := proto.Marshal(&pb.Reference{
		App:       &k.kc.AppID,
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
	if k.kc.AppID < other.kc.AppID {
		return true
	} else if k.kc.AppID > other.kc.AppID {
		return false
	}

	if k.kc.Namespace < other.kc.Namespace {
		return true
	} else if k.kc.Namespace > other.kc.Namespace {
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
	if !k.kc.Matches(other.kc) {
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
	fmt.Fprintf(ret, "KEY(DATASET(%s)", gqlQuoteString(k.kc.AppID))
	if ns := k.kc.Namespace; ns != "" {
		fmt.Fprintf(ret, ", NAMESPACE(%s)", gqlQuoteString(ns))
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
func (k *Key) Equal(other *Key) bool {
	return k.IncompleteEqual(other) && (k.LastTok() == other.LastTok())
}

// IncompleteEqual asserts that, were the two keys incomplete, they would be
// equal.
//
// This asserts equality for the full lineage of the key, except for its last
// token ID.
func (k *Key) IncompleteEqual(other *Key) (ret bool) {
	ret = (k.kc.Matches(other.kc) &&
		len(k.toks) == len(other.toks))
	if ret {
		for i, t := range k.toks {
			if i == len(k.toks)-1 {
				// Last token: check only Kind.
				if ret = (t.Kind == other.toks[i].Kind); !ret {
					return
				}
			} else {
				if ret = t == other.toks[i]; !ret {
					return
				}
			}
		}
	}
	return
}

// Incomplete returns an incomplete version of the key. The ID fields of the
// last token will be set to zero/empty.
func (k *Key) Incomplete() *Key {
	if k.IsIncomplete() {
		return k
	}
	return k.kc.NewKey(k.Kind(), "", 0, k.Parent())
}

// WithID returns the key generated by setting the ID of its last token to
// the specified value.
//
// To generate this, k is reduced to its Incomplete form, then populated with a
// new ID. The resulting key will have the same token linage as k (i.e., will
// be IncompleteEqual).
func (k *Key) WithID(stringID string, intID int64) *Key {
	if k.StringID() == stringID && k.IntID() == intID {
		return k
	}
	return k.kc.NewKey(k.Kind(), stringID, intID, k.Parent())
}

// Split componentizes the key into pieces (AppID, Namespace and tokens)
//
// Each token represents one piece of they key's 'path'.
//
// toks is guaranteed to be empty if and only if k is nil. If k is non-nil then
// it contains at least one token.
func (k *Key) Split() (appID, namespace string, toks []KeyTok) {
	appID = k.kc.AppID
	namespace = k.kc.Namespace
	toks = make([]KeyTok, len(k.toks))
	copy(toks, k.toks)
	return
}

// EstimateSize estimates the size of a Key.
//
// It uses https://cloud.google.com/appengine/articles/storage_breakdown?csw=1
// as a guide for these values.
func (k *Key) EstimateSize() int64 {
	ret := int64(len(k.kc.AppID))
	ret += int64(len(k.kc.Namespace))
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
