// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"errors"
	"fmt"
	"infra/gae/libs/wrapper"
	"strings"

	"github.com/mjibson/goon"
	"golang.org/x/net/context"

	"appengine/datastore"
	"appengine_internal"
	pb "appengine_internal/datastore"
)

//////////////////////////////////// public ////////////////////////////////////

// UseDS adds a wrapper.Datastore implementation to context, accessible
// by wrapper.GetDS(c)
func UseDS(c context.Context) context.Context {
	return wrapper.SetDSFactory(c, func(ic context.Context) wrapper.Datastore {
		dsd := cur(ic).Get(memContextDSIdx)

		switch x := dsd.(type) {
		case *dataStoreData:
			return &dsImpl{wrapper.DummyDS(), x, curGID(ic).namespace, ic}
		case *txnDataStoreData:
			return &txnDsImpl{wrapper.DummyDS(), x, curGID(ic).namespace}
		default:
			panic(fmt.Errorf("DS: bad type: %v in context %v", dsd, ic))
		}
	})
}

//////////////////////////////////// dsImpl ////////////////////////////////////

// dsImpl exists solely to bind the current c to the datastore data.
type dsImpl struct {
	wrapper.Datastore

	data *dataStoreData
	ns   string
	c    context.Context
}

var (
	_ = wrapper.Datastore((*dsImpl)(nil))
	_ = wrapper.Testable((*dsImpl)(nil))
)

func (d *dsImpl) BreakFeatures(err error, features ...string) {
	d.data.BreakFeatures(err, features...)
}
func (d *dsImpl) UnbreakFeatures(features ...string) {
	d.data.UnbreakFeatures(features...)
}

func (d *dsImpl) Kind(src interface{}) string {
	return kind(d.ns, d.KindNameResolver(), src)
}

func (d *dsImpl) KindNameResolver() goon.KindNameResolver {
	return d.data.KindNameResolver()
}
func (d *dsImpl) SetKindNameResolver(knr goon.KindNameResolver) {
	d.data.SetKindNameResolver(knr)
}

func (d *dsImpl) NewKey(kind, stringID string, intID int64, parent *datastore.Key) *datastore.Key {
	return newKey(d.ns, kind, stringID, intID, parent)
}
func (d *dsImpl) NewKeyObj(src interface{}) *datastore.Key {
	return newKeyObj(d.ns, d.KindNameResolver(), src)
}
func (d *dsImpl) NewKeyObjError(src interface{}) (*datastore.Key, error) {
	return newKeyObjError(d.ns, d.KindNameResolver(), src)
}

func (d *dsImpl) Put(src interface{}) (*datastore.Key, error) {
	if err := d.data.IsBroken(); err != nil {
		return nil, err
	}
	return d.data.put(d.ns, src)
}

func (d *dsImpl) Get(dst interface{}) error {
	if err := d.data.IsBroken(); err != nil {
		return err
	}
	return d.data.get(d.ns, dst)
}

func (d *dsImpl) Delete(key *datastore.Key) error {
	if err := d.data.IsBroken(); err != nil {
		return err
	}
	return d.data.del(d.ns, key)
}

////////////////////////////////// txnDsImpl ///////////////////////////////////

type txnDsImpl struct {
	wrapper.Datastore

	data *txnDataStoreData
	ns   string
}

var (
	_ = wrapper.Datastore((*txnDsImpl)(nil))
	_ = wrapper.Testable((*txnDsImpl)(nil))
)

func (d *txnDsImpl) BreakFeatures(err error, features ...string) {
	d.data.BreakFeatures(err, features...)
}
func (d *txnDsImpl) UnbreakFeatures(features ...string) {
	d.data.UnbreakFeatures(features...)
}

func (d *txnDsImpl) Kind(src interface{}) string {
	return kind(d.ns, d.KindNameResolver(), src)
}

func (d *txnDsImpl) KindNameResolver() goon.KindNameResolver {
	return d.data.KindNameResolver()
}
func (d *txnDsImpl) SetKindNameResolver(knr goon.KindNameResolver) {
	d.data.SetKindNameResolver(knr)
}

func (d *txnDsImpl) NewKey(kind, stringID string, intID int64, parent *datastore.Key) *datastore.Key {
	return newKey(d.ns, kind, stringID, intID, parent)
}
func (d *txnDsImpl) NewKeyObj(src interface{}) *datastore.Key {
	return newKeyObj(d.ns, d.KindNameResolver(), src)
}
func (d *txnDsImpl) NewKeyObjError(src interface{}) (*datastore.Key, error) {
	return newKeyObjError(d.ns, d.KindNameResolver(), src)
}

func (d *txnDsImpl) Put(src interface{}) (*datastore.Key, error) {
	if err := d.data.IsBroken(); err != nil {
		return nil, err
	}
	return d.data.put(d.ns, src)
}

func (d *txnDsImpl) Get(dst interface{}) error {
	if err := d.data.IsBroken(); err != nil {
		return err
	}
	return d.data.get(d.ns, dst)
}

func (d *txnDsImpl) Delete(key *datastore.Key) error {
	if err := d.data.IsBroken(); err != nil {
		return err
	}
	return d.data.del(d.ns, key)
}

func (*txnDsImpl) RunInTransaction(func(c context.Context) error, *datastore.TransactionOptions) error {
	return errors.New("datastore: nested transactions are not supported")
}

////////////////////////////// private functions ///////////////////////////////

func newDSError(code pb.Error_ErrorCode, message ...string) *appengine_internal.APIError {
	return &appengine_internal.APIError{
		Detail:  strings.Join(message, ""),
		Service: "datastore_v3",
		Code:    int32(code),
	}
}
