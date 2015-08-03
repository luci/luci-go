// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// adapted from github.com/golang/appengine/datastore

package datastore

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"

	pb "github.com/luci/gae/service/datastore/internal/protos/datastore"

	"github.com/golang/protobuf/proto"
)

// KeyEncode encodes the provided key as a base64-encoded protobuf.
//
// This encoding is compatible with the SDK-provided encoding and is agnostic
// to the underlying implementation of the Key.
func KeyEncode(k Key) string {
	n := 0
	for i := k; i != nil; i = i.Parent() {
		n++
	}
	e := make([]*pb.Path_Element, n)
	for i := k; i != nil; i = i.Parent() {
		n--
		kind := i.Kind()
		e[n] = &pb.Path_Element{
			Type: &kind,
		}
		// At most one of {Name,Id} should be set.
		// Neither will be set for incomplete keys.
		if i.StringID() != "" {
			sid := i.StringID()
			e[n].Name = &sid
		} else if i.IntID() != 0 {
			iid := i.IntID()
			e[n].Id = &iid
		}
	}
	var namespace *string
	if k.Namespace() != "" {
		namespace = proto.String(k.Namespace())
	}
	r, err := proto.Marshal(&pb.Reference{
		App:       proto.String(k.AppID()),
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

// KeyToksDecode decodes a base64-encoded protobuf representation of a Key
// into a tokenized form. This is so that implementations of the gae wrapper
// can decode to their own implementation of Key.
//
// This encoding is compatible with the SDK-provided encoding and is agnostic
// to the underlying implementation of the Key.
func KeyToksDecode(encoded string) (appID, namespace string, toks []KeyTok, err error) {
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

	appID = r.GetApp()
	namespace = r.GetNameSpace()
	toks = make([]KeyTok, len(r.Path.Element))
	for i, e := range r.Path.Element {
		toks[i] = KeyTok{
			Kind:     e.GetType(),
			IntID:    e.GetId(),
			StringID: e.GetName(),
		}
	}
	return
}

// KeyMarshalJSON returns a MarshalJSON-compatible serialization of a Key.
func KeyMarshalJSON(k Key) ([]byte, error) {
	return []byte(`"` + KeyEncode(k) + `"`), nil
}

// KeyUnmarshalJSON returns the tokenized version of a Key as encoded by
// KeyMarshalJSON.
func KeyUnmarshalJSON(buf []byte) (appID, namespace string, toks []KeyTok, err error) {
	if len(buf) < 2 || buf[0] != '"' || buf[len(buf)-1] != '"' {
		err = errors.New("datastore: bad JSON key")
	} else {
		appID, namespace, toks, err = KeyToksDecode(string(buf[1 : len(buf)-1]))
	}
	return
}

// KeyIncomplete returns true iff k doesn't have an id yet.
func KeyIncomplete(k Key) bool {
	return k != nil && k.StringID() == "" && k.IntID() == 0
}

// KeyValid determines if a key is valid, according to a couple rules:
//   - k is not nil
//   - every token of k:
//     - (if !allowSpecial) token's kind doesn't start with '__'
//     - token's kind and appid are non-blank
//     - token is not incomplete
//   - all tokens have the same namespace and appid
func KeyValid(k Key, allowSpecial bool, aid, ns string) bool {
	if k == nil {
		return false
	}
	if aid != k.AppID() || ns != k.Namespace() {
		return false
	}
	for ; k != nil; k = k.Parent() {
		if !allowSpecial && len(k.Kind()) >= 2 && k.Kind()[:2] == "__" {
			return false
		}
		if k.Kind() == "" || k.AppID() == "" {
			return false
		}
		if k.StringID() != "" && k.IntID() != 0 {
			return false
		}
		if k.Parent() != nil {
			if KeyIncomplete(k.Parent()) {
				return false
			}
			if k.Parent().AppID() != k.AppID() || k.Parent().Namespace() != k.Namespace() {
				return false
			}
		}
	}
	return true
}

// KeyRoot returns the entity root for the given key.
func KeyRoot(k Key) Key {
	for k != nil && k.Parent() != nil {
		k = k.Parent()
	}
	return k
}

// KeysEqual returns true iff the two keys represent identical key values.
func KeysEqual(a, b Key) (ret bool) {
	ret = (a.Kind() == b.Kind() &&
		a.StringID() == b.StringID() &&
		a.IntID() == b.IntID() &&
		a.AppID() == b.AppID() &&
		a.Namespace() == b.Namespace())
	if !ret {
		return
	}
	ap, bp := a.Parent(), b.Parent()
	return (ap == nil && bp == nil) || KeysEqual(ap, bp)
}

func marshalDSKey(b *bytes.Buffer, k Key) {
	if k.Parent() != nil {
		marshalDSKey(b, k.Parent())
	}
	b.WriteByte('/')
	b.WriteString(k.Kind())
	b.WriteByte(',')
	if k.StringID() != "" {
		b.WriteString(k.StringID())
	} else {
		b.WriteString(strconv.FormatInt(k.IntID(), 10))
	}
}

// KeyString returns a human-readable representation of the key, and is the
// typical implementation of Key.String() (though it isn't guaranteed to be)
func KeyString(k Key) string {
	if k == nil {
		return ""
	}
	b := bytes.NewBuffer(make([]byte, 0, 512))
	marshalDSKey(b, k)
	return b.String()
}

// KeySplit splits the key into its constituent parts. Note that if the key is
// not KeyValid, this method may not provide a round-trip for k.
func KeySplit(k Key) (appID, namespace string, toks []KeyTok) {
	if k == nil {
		return
	}

	if sk, ok := k.(*GenericKey); ok {
		if sk == nil {
			return
		}
		return sk.appID, sk.namespace, sk.toks
	}

	n := 0
	for i := k; i != nil; i = i.Parent() {
		n++
	}
	toks = make([]KeyTok, n)
	for i := k; i != nil; i = i.Parent() {
		n--
		toks[n].IntID = i.IntID()
		toks[n].StringID = i.StringID()
		toks[n].Kind = i.Kind()
	}
	appID = k.AppID()
	namespace = k.Namespace()
	return
}
