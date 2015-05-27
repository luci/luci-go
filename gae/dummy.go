// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wrapper

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"golang.org/x/net/context"

	"appengine"
	"appengine/datastore"
	"appengine/memcache"
	"appengine/taskqueue"

	"github.com/mjibson/goon"
)

const niFmtStr = "wrapper: method %s.%s is not implemented"

func ni() error {
	iface := "UNKNOWN"
	funcName := "UNKNOWN"

	if ptr, _, _, ok := runtime.Caller(1); ok {
		f := runtime.FuncForPC(ptr)
		n := f.Name()
		if n != "" {
			parts := strings.Split(n, ".")
			if len(parts) == 3 {
				switch parts[1][len(parts[1])-2:] {
				case "DS":
					iface = "Datastore"
				case "MC":
					iface = "Memcache"
				case "TQ":
					iface = "TaskQueue"
				case "GI":
					iface = "GlobalInformation"
				case "QY":
					iface = "Query"
				}
				funcName = parts[2]
			}
		}
	}

	return fmt.Errorf(niFmtStr, iface, funcName)
}

/////////////////////////////////// dummyDS ////////////////////////////////////

type dummyDS struct{}

func (dummyDS) Kind(interface{}) string                                     { panic(ni()) }
func (dummyDS) KindNameResolver() goon.KindNameResolver                     { panic(ni()) }
func (dummyDS) SetKindNameResolver(goon.KindNameResolver)                   { panic(ni()) }
func (dummyDS) NewKey(string, string, int64, *datastore.Key) *datastore.Key { panic(ni()) }
func (dummyDS) NewKeyObj(interface{}) *datastore.Key                        { panic(ni()) }
func (dummyDS) NewKeyObjError(interface{}) (*datastore.Key, error)          { panic(ni()) }
func (dummyDS) Put(interface{}) (*datastore.Key, error)                     { panic(ni()) }
func (dummyDS) Get(interface{}) error                                       { panic(ni()) }
func (dummyDS) Delete(*datastore.Key) error                                 { panic(ni()) }
func (dummyDS) PutMulti(interface{}) ([]*datastore.Key, error)              { panic(ni()) }
func (dummyDS) GetMulti(interface{}) error                                  { panic(ni()) }
func (dummyDS) DeleteMulti([]*datastore.Key) error                          { panic(ni()) }
func (dummyDS) NewQuery(string) DSQuery                                     { panic(ni()) }
func (dummyDS) Run(DSQuery) DSIterator                                      { panic(ni()) }
func (dummyDS) GetAll(DSQuery, interface{}) ([]*datastore.Key, error)       { panic(ni()) }
func (dummyDS) Count(DSQuery) (int, error)                                  { panic(ni()) }
func (dummyDS) RunInTransaction(func(context.Context) error, *datastore.TransactionOptions) error {
	panic(ni())
}

var dummyDSInst = dummyDS{}

// DummyDS returns a dummy Datastore implementation suitable for embedding.
// Every method panics with a message containing the name of the method which
// was unimplemented.
func DummyDS() Datastore { return dummyDSInst }

/////////////////////////////////// dummyMC ////////////////////////////////////

type dummyMC struct{}

func (dummyMC) Add(*memcache.Item) error                             { panic(ni()) }
func (dummyMC) Set(*memcache.Item) error                             { panic(ni()) }
func (dummyMC) Get(string) (*memcache.Item, error)                   { panic(ni()) }
func (dummyMC) Delete(string) error                                  { panic(ni()) }
func (dummyMC) CompareAndSwap(*memcache.Item) error                  { panic(ni()) }
func (dummyMC) AddMulti([]*memcache.Item) error                      { panic(ni()) }
func (dummyMC) SetMulti([]*memcache.Item) error                      { panic(ni()) }
func (dummyMC) GetMulti([]string) (map[string]*memcache.Item, error) { panic(ni()) }
func (dummyMC) DeleteMulti([]string) error                           { panic(ni()) }
func (dummyMC) CompareAndSwapMulti([]*memcache.Item) error           { panic(ni()) }
func (dummyMC) Increment(string, int64, uint64) (uint64, error)      { panic(ni()) }
func (dummyMC) IncrementExisting(string, int64) (uint64, error)      { panic(ni()) }
func (dummyMC) Flush() error                                         { panic(ni()) }
func (dummyMC) Stats() (*memcache.Statistics, error)                 { panic(ni()) }
func (dummyMC) InflateCodec(memcache.Codec) MCCodec                  { panic(ni()) }

var dummyMCInst = dummyMC{}

// DummyMC returns a dummy Memcache implementation suitable for embedding.
// Every method panics with a message containing the name of the method which
// was unimplemented.
func DummyMC() Memcache { return dummyMCInst }

/////////////////////////////////// dummyTQ ////////////////////////////////////

type dummyTQ struct{}

func (dummyTQ) Add(*taskqueue.Task, string) (*taskqueue.Task, error)           { panic(ni()) }
func (dummyTQ) Delete(*taskqueue.Task, string) error                           { panic(ni()) }
func (dummyTQ) AddMulti([]*taskqueue.Task, string) ([]*taskqueue.Task, error)  { panic(ni()) }
func (dummyTQ) DeleteMulti([]*taskqueue.Task, string) error                    { panic(ni()) }
func (dummyTQ) Lease(int, string, int) ([]*taskqueue.Task, error)              { panic(ni()) }
func (dummyTQ) LeaseByTag(int, string, int, string) ([]*taskqueue.Task, error) { panic(ni()) }
func (dummyTQ) ModifyLease(*taskqueue.Task, string, int) error                 { panic(ni()) }
func (dummyTQ) Purge(string) error                                             { panic(ni()) }
func (dummyTQ) QueueStats([]string, int) ([]taskqueue.QueueStatistics, error)  { panic(ni()) }

var dummyTQInst = dummyTQ{}

// DummyTQ returns a dummy TaskQueue implementation suitable for embedding.
// Every method panics with a message containing the name of the method which
// was unimplemented.
func DummyTQ() TaskQueue { return dummyTQInst }

/////////////////////////////////// dummyQY ////////////////////////////////////

type dummyQY struct{}

func (dummyQY) Ancestor(ancestor *datastore.Key) DSQuery           { panic(ni()) }
func (dummyQY) Distinct() DSQuery                                  { panic(ni()) }
func (dummyQY) End(c DSCursor) DSQuery                             { panic(ni()) }
func (dummyQY) EventualConsistency() DSQuery                       { panic(ni()) }
func (dummyQY) Filter(filterStr string, value interface{}) DSQuery { panic(ni()) }
func (dummyQY) KeysOnly() DSQuery                                  { panic(ni()) }
func (dummyQY) Limit(limit int) DSQuery                            { panic(ni()) }
func (dummyQY) Offset(offset int) DSQuery                          { panic(ni()) }
func (dummyQY) Order(fieldName string) DSQuery                     { panic(ni()) }
func (dummyQY) Project(fieldNames ...string) DSQuery               { panic(ni()) }
func (dummyQY) Start(c DSCursor) DSQuery                           { panic(ni()) }

var dummyQYInst = dummyQY{}

// DummyQY returns a dummy DSQuery implementation suitable for embedding.
// Every method panics with a message containing the name of the method which
// was unimplemented.
func DummyQY() DSQuery { return dummyQYInst }

/////////////////////////////////// dummyGI ////////////////////////////////////

type dummyGI struct{}

func (dummyGI) AccessToken(scopes ...string) (token string, expiry time.Time, err error) { panic(ni()) }
func (dummyGI) AppID() string                                                            { panic(ni()) }
func (dummyGI) ModuleHostname(module, version, instance string) (string, error)          { panic(ni()) }
func (dummyGI) ModuleName() string                                                       { panic(ni()) }
func (dummyGI) DefaultVersionHostname() string                                           { panic(ni()) }
func (dummyGI) PublicCertificates() ([]appengine.Certificate, error)                     { panic(ni()) }
func (dummyGI) RequestID() string                                                        { panic(ni()) }
func (dummyGI) ServiceAccount() (string, error)                                          { panic(ni()) }
func (dummyGI) SignBytes(bytes []byte) (keyName string, signature []byte, err error)     { panic(ni()) }
func (dummyGI) VersionID() string                                                        { panic(ni()) }
func (dummyGI) Namespace(namespace string) (context.Context, error)                      { panic(ni()) }
func (dummyGI) Datacenter() string                                                       { panic(ni()) }
func (dummyGI) InstanceID() string                                                       { panic(ni()) }
func (dummyGI) IsDevAppserver() bool                                                     { panic(ni()) }
func (dummyGI) ServerSoftware() string                                                   { panic(ni()) }
func (dummyGI) IsCapabilityDisabled(err error) bool                                      { panic(ni()) }
func (dummyGI) IsOverQuota(err error) bool                                               { panic(ni()) }
func (dummyGI) IsTimeoutError(err error) bool                                            { panic(ni()) }

var dummyGIInst = dummyGI{}

// DummyGI returns a dummy GlobalInfo implementation suitable for embedding.
// Every method panics with a message containing the name of the method which
// was unimplemented.
func DummyGI() GlobalInfo { return dummyGIInst }
