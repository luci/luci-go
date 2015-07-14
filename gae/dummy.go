// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gae

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"golang.org/x/net/context"
)

const niFmtStr = "gae: method %s.%s is not implemented"

func ni() error {
	iface := "UNKNOWN"
	funcName := "UNKNOWN"

	if ptr, _, _, ok := runtime.Caller(1); ok {
		f := runtime.FuncForPC(ptr)
		n := f.Name()
		if n != "" {
			parts := strings.Split(n, ".")
			if len(parts) == 3 {
				switch parts[1][len("dummy"):] {
				case "RDS":
					iface = "RawDatastore"
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

/////////////////////////////////// dummyRDS ////////////////////////////////////

type dummyRDS struct{}

func (dummyRDS) NewKey(string, string, int64, DSKey) DSKey              { panic(ni()) }
func (dummyRDS) DecodeKey(string) (DSKey, error)                        { panic(ni()) }
func (dummyRDS) KeyFromTokens(a, n string, t []DSKeyTok) (DSKey, error) { panic(ni()) }
func (dummyRDS) Put(DSKey, interface{}) (DSKey, error)                  { panic(ni()) }
func (dummyRDS) Get(DSKey, interface{}) error                           { panic(ni()) }
func (dummyRDS) Delete(DSKey) error                                     { panic(ni()) }
func (dummyRDS) PutMulti([]DSKey, interface{}) ([]DSKey, error)         { panic(ni()) }
func (dummyRDS) GetMulti([]DSKey, interface{}) error                    { panic(ni()) }
func (dummyRDS) DeleteMulti([]DSKey) error                              { panic(ni()) }
func (dummyRDS) NewQuery(string) DSQuery                                { panic(ni()) }
func (dummyRDS) Run(DSQuery) DSIterator                                 { panic(ni()) }
func (dummyRDS) GetAll(DSQuery, interface{}) ([]DSKey, error)           { panic(ni()) }
func (dummyRDS) Count(DSQuery) (int, error)                             { panic(ni()) }
func (dummyRDS) RunInTransaction(func(context.Context) error, *DSTransactionOptions) error {
	panic(ni())
}

var dummyRDSInst = dummyRDS{}

// DummyRDS returns a dummy RawDatastore implementation suitable for embedding.
// Every method panics with a message containing the name of the method which
// was unimplemented.
func DummyRDS() RawDatastore { return dummyRDSInst }

/////////////////////////////////// dummyMC ////////////////////////////////////

type dummyMC struct{}

func (dummyMC) Add(MCItem) error                                { panic(ni()) }
func (dummyMC) NewItem(key string) MCItem                       { panic(ni()) }
func (dummyMC) Set(MCItem) error                                { panic(ni()) }
func (dummyMC) Get(string) (MCItem, error)                      { panic(ni()) }
func (dummyMC) Delete(string) error                             { panic(ni()) }
func (dummyMC) CompareAndSwap(MCItem) error                     { panic(ni()) }
func (dummyMC) AddMulti([]MCItem) error                         { panic(ni()) }
func (dummyMC) SetMulti([]MCItem) error                         { panic(ni()) }
func (dummyMC) GetMulti([]string) (map[string]MCItem, error)    { panic(ni()) }
func (dummyMC) DeleteMulti([]string) error                      { panic(ni()) }
func (dummyMC) CompareAndSwapMulti([]MCItem) error              { panic(ni()) }
func (dummyMC) Increment(string, int64, uint64) (uint64, error) { panic(ni()) }
func (dummyMC) IncrementExisting(string, int64) (uint64, error) { panic(ni()) }
func (dummyMC) Flush() error                                    { panic(ni()) }
func (dummyMC) Stats() (*MCStatistics, error)                   { panic(ni()) }

var dummyMCInst = dummyMC{}

// DummyMC returns a dummy Memcache implementation suitable for embedding.
// Every method panics with a message containing the name of the method which
// was unimplemented.
func DummyMC() Memcache { return dummyMCInst }

/////////////////////////////////// dummyTQ ////////////////////////////////////

type dummyTQ struct{}

func (dummyTQ) Add(*TQTask, string) (*TQTask, error)                   { panic(ni()) }
func (dummyTQ) Delete(*TQTask, string) error                           { panic(ni()) }
func (dummyTQ) AddMulti([]*TQTask, string) ([]*TQTask, error)          { panic(ni()) }
func (dummyTQ) DeleteMulti([]*TQTask, string) error                    { panic(ni()) }
func (dummyTQ) Lease(int, string, int) ([]*TQTask, error)              { panic(ni()) }
func (dummyTQ) LeaseByTag(int, string, int, string) ([]*TQTask, error) { panic(ni()) }
func (dummyTQ) ModifyLease(*TQTask, string, int) error                 { panic(ni()) }
func (dummyTQ) Purge(string) error                                     { panic(ni()) }
func (dummyTQ) QueueStats([]string) ([]TQStatistics, error)            { panic(ni()) }

var dummyTQInst = dummyTQ{}

// DummyTQ returns a dummy TaskQueue implementation suitable for embedding.
// Every method panics with a message containing the name of the method which
// was unimplemented.
func DummyTQ() TaskQueue { return dummyTQInst }

/////////////////////////////////// dummyQY ////////////////////////////////////

type dummyQY struct{}

func (dummyQY) Ancestor(ancestor DSKey) DSQuery                    { panic(ni()) }
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
func (dummyGI) PublicCertificates() ([]GICertificate, error)                             { panic(ni()) }
func (dummyGI) RequestID() string                                                        { panic(ni()) }
func (dummyGI) ServiceAccount() (string, error)                                          { panic(ni()) }
func (dummyGI) SignBytes(bytes []byte) (keyName string, signature []byte, err error)     { panic(ni()) }
func (dummyGI) VersionID() string                                                        { panic(ni()) }
func (dummyGI) Namespace(namespace string) (context.Context, error)                      { panic(ni()) }
func (dummyGI) Datacenter() string                                                       { panic(ni()) }
func (dummyGI) InstanceID() string                                                       { panic(ni()) }
func (dummyGI) IsDevAppServer() bool                                                     { panic(ni()) }
func (dummyGI) ServerSoftware() string                                                   { panic(ni()) }
func (dummyGI) IsCapabilityDisabled(err error) bool                                      { panic(ni()) }
func (dummyGI) IsOverQuota(err error) bool                                               { panic(ni()) }
func (dummyGI) IsTimeoutError(err error) bool                                            { panic(ni()) }

var dummyGIInst = dummyGI{}

// DummyGI returns a dummy GlobalInfo implementation suitable for embedding.
// Every method panics with a message containing the name of the method which
// was unimplemented.
func DummyGI() GlobalInfo { return dummyGIInst }
