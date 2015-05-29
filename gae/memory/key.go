// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/funnybase"
	"github.com/mjibson/goon"

	"appengine"
	"appengine/datastore"
	"appengine_internal"
	basepb "appengine_internal/base"
)

const keyNumToksReasonableLimit = 50

///////////////////////////// fakeGAECtxForNewKey //////////////////////////////

type fakeGAECtxForNewKey string

var _ = appengine.Context((fakeGAECtxForNewKey)(""))

func (fakeGAECtxForNewKey) Debugf(format string, args ...interface{})    {}
func (fakeGAECtxForNewKey) Infof(format string, args ...interface{})     {}
func (fakeGAECtxForNewKey) Warningf(format string, args ...interface{})  {}
func (fakeGAECtxForNewKey) Errorf(format string, args ...interface{})    {}
func (fakeGAECtxForNewKey) Criticalf(format string, args ...interface{}) {}
func (fakeGAECtxForNewKey) Request() interface{}                         { return nil }
func (f fakeGAECtxForNewKey) Call(service, method string, in, out appengine_internal.ProtoMessage, opts *appengine_internal.CallOptions) error {
	if service != "__go__" || method != "GetNamespace" {
		panic(fmt.Errorf("fakeGAECtxForNewKey: cannot facilitate Call(%q, %q, ...)", service, method))
	}
	out.(*basepb.StringProto).Value = proto.String(string(f))
	return nil
}
func (fakeGAECtxForNewKey) FullyQualifiedAppID() string { return "dev~my~app" }

/////////////////////////////// Key construction ///////////////////////////////

func newKey(ns, kind, stringID string, intID int64, parent *datastore.Key) *datastore.Key {
	return datastore.NewKey(fakeGAECtxForNewKey(ns), kind, stringID, intID, parent)
}
func newKeyObjError(ns string, knr goon.KindNameResolver, src interface{}) (*datastore.Key, error) {
	return (&goon.Goon{
		Context:          fakeGAECtxForNewKey(ns),
		KindNameResolver: knr}).KeyError(src)
}
func newKeyObj(ns string, knr goon.KindNameResolver, obj interface{}) *datastore.Key {
	k, err := newKeyObjError(ns, knr, obj)
	if err != nil {
		panic(err)
	}
	return k
}
func kind(ns string, knr goon.KindNameResolver, src interface{}) string {
	return newKeyObj(ns, knr, src).Kind()
}

/////////////////////////////// Binary Encoding ////////////////////////////////

type keyTok struct {
	kind     string
	intID    uint64
	stringID string
}

func keyToToks(key *datastore.Key) (namespace string, ret []*keyTok) {
	var inner func(*datastore.Key)
	inner = func(k *datastore.Key) {
		if k.Parent() != nil {
			inner(k.Parent())
		}
		ret = append(ret, &keyTok{k.Kind(), uint64(k.IntID()), k.StringID()})
	}
	inner(key)
	namespace = key.Namespace()
	return
}

func toksToKey(ns string, toks []*keyTok) (ret *datastore.Key) {
	for _, t := range toks {
		ret = newKey(ns, t.kind, t.stringID, int64(t.intID), ret)
	}
	return
}

type nsOption bool

const (
	withNS nsOption = true
	noNS            = false
)

func keyBytes(nso nsOption, k *datastore.Key) []byte {
	buf := &bytes.Buffer{}
	writeKey(buf, nso, k)
	return buf.Bytes()
}

func keyFromByteString(nso nsOption, d string) (*datastore.Key, error) {
	return readKey(bytes.NewBufferString(d), nso)
}

func writeKey(buf *bytes.Buffer, nso nsOption, k *datastore.Key) {
	// namespace ++ #tokens ++ [tok.kind ++ tok.stringID ++ tok.intID?]*
	namespace, toks := keyToToks(k)
	if nso == withNS {
		writeString(buf, namespace)
	}
	funnybase.WriteUint(buf, uint64(len(toks)))
	for _, tok := range toks {
		writeString(buf, tok.kind)
		writeString(buf, tok.stringID)
		if tok.stringID == "" {
			funnybase.WriteUint(buf, tok.intID)
		}
	}
}

func readKey(buf *bytes.Buffer, nso nsOption) (*datastore.Key, error) {
	namespace := ""
	if nso == withNS {
		err := error(nil)
		if namespace, err = readString(buf); err != nil {
			return nil, err
		}
	}

	numToks, err := funnybase.ReadUint(buf)
	if err != nil {
		return nil, err
	}
	if numToks > keyNumToksReasonableLimit {
		return nil, fmt.Errorf("readKey: tried to decode huge key of length %d", numToks)
	}

	toks := make([]*keyTok, numToks)
	for i := uint64(0); i < numToks; i++ {
		tok := &keyTok{}
		if tok.kind, err = readString(buf); err != nil {
			return nil, err
		}
		if tok.stringID, err = readString(buf); err != nil {
			return nil, err
		}
		if tok.stringID == "" {
			if tok.intID, err = funnybase.ReadUint(buf); err != nil {
				return nil, err
			}
			if tok.intID == 0 {
				return nil, errors.New("readKey: decoded key with empty stringID and empty intID")
			}
		}
		toks[i] = tok
	}

	return toksToKey(namespace, toks), nil
}

//////////////////////////////// Key utilities /////////////////////////////////

func rootKey(key *datastore.Key) *datastore.Key {
	for key.Parent() != nil {
		key = key.Parent()
	}
	return key
}

type keyValidOption bool

const (
	// UserKeyOnly is used with KeyValid, and ensures that the key is only one
	// that's valid for a user program to write to.
	UserKeyOnly keyValidOption = false

	// AllowSpecialKeys is used with KeyValid, and allows keys for special
	// metadata objects (like "__entity_group__").
	AllowSpecialKeys = true
)

// KeyValid checks to see if a key is valid by applying a bunch of constraint
// rules to it (e.g. can't have StringID and IntID set at the same time, can't
// have a parent key which is Incomplete(), etc.)
//
// It verifies that the key is also in the provided namespace. It can also
// reject keys which are 'special' e.g. have a Kind starting with "__". This
// behavior is controllable with opt.
func KeyValid(ns string, k *datastore.Key, opt keyValidOption) bool {
	// copied from the appengine SDK because why would any user program need to
	// see if a key is valid? /s
	if k == nil {
		return false
	}
	// since we do "client-side" validation of namespaces, check this here.
	if k.Namespace() != ns {
		return false
	}
	for ; k != nil; k = k.Parent() {
		if opt == UserKeyOnly && len(k.Kind()) >= 2 && k.Kind()[:2] == "__" { // reserve all Kinds starting with __
			return false
		}
		if k.Kind() == "" || k.AppID() == "" {
			return false
		}
		if k.StringID() != "" && k.IntID() != 0 {
			return false
		}
		if k.Parent() != nil {
			if k.Parent().Incomplete() {
				return false
			}
			if k.Parent().AppID() != k.AppID() || k.Parent().Namespace() != k.Namespace() {
				return false
			}
		}
	}
	return true
}

// KeyCouldBeValid is like KeyValid, but it allows for the possibility that the
// last token of the key is Incomplete(). It returns true if the Key will become
// valid once it recieves an automatically-assigned ID.
func KeyCouldBeValid(ns string, k *datastore.Key, opt keyValidOption) bool {
	// adds an id to k if it's incomplete.
	if k.Incomplete() {
		k = newKey(ns, k.Kind(), "", 1, k.Parent())
	}
	return KeyValid(ns, k, opt)
}
