// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dummy

import (
	"fmt"
	"golang.org/x/net/context"
	"runtime"
	"strings"
	"time"

	"infra/gae/libs/gae"
)

const niFmtStr = "dummy: method %s.%s is not implemented"

// ni returns an error whose message is an appropriate expansion of niFmtStr.
//
// It walks the stack to find out what interface and method it's being
// called from. For example, it might return a message which looks like:
//   dummy: method RawDatastore.Get is not implemented
//
// This allows the various dummy objects below to have clear boilerplate which
// avoids copy+paste errors (such as if each one of them filled in the template
// manually).
//
// If this function is somehow called from something other than one of the dummy
// objects in this package, it will substitute the string UNKNOWN for the
// interface and/or the method in the niFmtStr template.
func ni() error {
	iface := "UNKNOWN"
	funcName := "UNKNOWN"

	if ptr, _, _, ok := runtime.Caller(1); ok {
		f := runtime.FuncForPC(ptr)
		n := f.Name()
		if n != "" {
			parts := strings.Split(n, ".")
			if len(parts) == 3 {
				switch parts[1] {
				case "rds":
					iface = "RawDatastore"
				case "mc":
					iface = "Memcache"
				case "tq":
					iface = "TaskQueue"
				case "gi":
					iface = "GlobalInfo"
				case "qy":
					iface = "DSQuery"
				}
				funcName = parts[2]
			}
		}
	}

	return fmt.Errorf(niFmtStr, iface, funcName)
}

/////////////////////////////////// rds ////////////////////////////////////

type rds struct{}

func (rds) NewKey(string, string, int64, gae.DSKey) gae.DSKey              { panic(ni()) }
func (rds) DecodeKey(string) (gae.DSKey, error)                            { panic(ni()) }
func (rds) KeyFromTokens(a, n string, t []gae.DSKeyTok) (gae.DSKey, error) { panic(ni()) }
func (rds) Put(gae.DSKey, interface{}) (gae.DSKey, error)                  { panic(ni()) }
func (rds) Get(gae.DSKey, interface{}) error                               { panic(ni()) }
func (rds) Delete(gae.DSKey) error                                         { panic(ni()) }
func (rds) PutMulti([]gae.DSKey, interface{}) ([]gae.DSKey, error)         { panic(ni()) }
func (rds) GetMulti([]gae.DSKey, interface{}) error                        { panic(ni()) }
func (rds) DeleteMulti([]gae.DSKey) error                                  { panic(ni()) }
func (rds) NewQuery(string) gae.DSQuery                                    { panic(ni()) }
func (rds) Run(gae.DSQuery) gae.DSIterator                                 { panic(ni()) }
func (rds) GetAll(gae.DSQuery, interface{}) ([]gae.DSKey, error)           { panic(ni()) }
func (rds) Count(gae.DSQuery) (int, error)                                 { panic(ni()) }
func (rds) RunInTransaction(func(context.Context) error, *gae.DSTransactionOptions) error {
	panic(ni())
}

var dummyRDSInst = rds{}

// RDS returns a dummy RawDatastore implementation suitable for embedding.
// Every method panics with a message containing the name of the method which
// was unimplemented.
func RDS() gae.RawDatastore { return dummyRDSInst }

/////////////////////////////////// mc ////////////////////////////////////

type mc struct{}

func (mc) Add(gae.MCItem) error                             { panic(ni()) }
func (mc) NewItem(key string) gae.MCItem                    { panic(ni()) }
func (mc) Set(gae.MCItem) error                             { panic(ni()) }
func (mc) Get(string) (gae.MCItem, error)                   { panic(ni()) }
func (mc) Delete(string) error                              { panic(ni()) }
func (mc) CompareAndSwap(gae.MCItem) error                  { panic(ni()) }
func (mc) AddMulti([]gae.MCItem) error                      { panic(ni()) }
func (mc) SetMulti([]gae.MCItem) error                      { panic(ni()) }
func (mc) GetMulti([]string) (map[string]gae.MCItem, error) { panic(ni()) }
func (mc) DeleteMulti([]string) error                       { panic(ni()) }
func (mc) CompareAndSwapMulti([]gae.MCItem) error           { panic(ni()) }
func (mc) Increment(string, int64, uint64) (uint64, error)  { panic(ni()) }
func (mc) IncrementExisting(string, int64) (uint64, error)  { panic(ni()) }
func (mc) Flush() error                                     { panic(ni()) }
func (mc) Stats() (*gae.MCStatistics, error)                { panic(ni()) }

var dummyMCInst = mc{}

// MC returns a dummy Memcache implementation suitable for embedding.
// Every method panics with a message containing the name of the method which
// was unimplemented.
func MC() gae.Memcache { return dummyMCInst }

/////////////////////////////////// tq ////////////////////////////////////

type tq struct{}

func (tq) Add(*gae.TQTask, string) (*gae.TQTask, error)               { panic(ni()) }
func (tq) Delete(*gae.TQTask, string) error                           { panic(ni()) }
func (tq) AddMulti([]*gae.TQTask, string) ([]*gae.TQTask, error)      { panic(ni()) }
func (tq) DeleteMulti([]*gae.TQTask, string) error                    { panic(ni()) }
func (tq) Lease(int, string, int) ([]*gae.TQTask, error)              { panic(ni()) }
func (tq) LeaseByTag(int, string, int, string) ([]*gae.TQTask, error) { panic(ni()) }
func (tq) ModifyLease(*gae.TQTask, string, int) error                 { panic(ni()) }
func (tq) Purge(string) error                                         { panic(ni()) }
func (tq) QueueStats([]string) ([]gae.TQStatistics, error)            { panic(ni()) }

var dummyTQInst = tq{}

// TQ returns a dummy TaskQueue implementation suitable for embedding.
// Every method panics with a message containing the name of the method which
// was unimplemented.
func TQ() gae.TaskQueue { return dummyTQInst }

/////////////////////////////////// qy ////////////////////////////////////

type qy struct{}

func (qy) Ancestor(ancestor gae.DSKey) gae.DSQuery                { panic(ni()) }
func (qy) Distinct() gae.DSQuery                                  { panic(ni()) }
func (qy) End(c gae.DSCursor) gae.DSQuery                         { panic(ni()) }
func (qy) EventualConsistency() gae.DSQuery                       { panic(ni()) }
func (qy) Filter(filterStr string, value interface{}) gae.DSQuery { panic(ni()) }
func (qy) KeysOnly() gae.DSQuery                                  { panic(ni()) }
func (qy) Limit(limit int) gae.DSQuery                            { panic(ni()) }
func (qy) Offset(offset int) gae.DSQuery                          { panic(ni()) }
func (qy) Order(fieldName string) gae.DSQuery                     { panic(ni()) }
func (qy) Project(fieldNames ...string) gae.DSQuery               { panic(ni()) }
func (qy) Start(c gae.DSCursor) gae.DSQuery                       { panic(ni()) }

var dummyQYInst = qy{}

// QY returns a dummy gae.DSQuery implementation suitable for embedding.
// Every method panics with a message containing the name of the method which
// was unimplemented.
func QY() gae.DSQuery { return dummyQYInst }

/////////////////////////////////// gi ////////////////////////////////////

type gi struct{}

func (gi) AccessToken(scopes ...string) (token string, expiry time.Time, err error) { panic(ni()) }
func (gi) AppID() string                                                            { panic(ni()) }
func (gi) ModuleHostname(module, version, instance string) (string, error)          { panic(ni()) }
func (gi) ModuleName() string                                                       { panic(ni()) }
func (gi) DefaultVersionHostname() string                                           { panic(ni()) }
func (gi) PublicCertificates() ([]gae.GICertificate, error)                         { panic(ni()) }
func (gi) RequestID() string                                                        { panic(ni()) }
func (gi) ServiceAccount() (string, error)                                          { panic(ni()) }
func (gi) SignBytes(bytes []byte) (keyName string, signature []byte, err error)     { panic(ni()) }
func (gi) VersionID() string                                                        { panic(ni()) }
func (gi) Namespace(namespace string) (context.Context, error)                      { panic(ni()) }
func (gi) Datacenter() string                                                       { panic(ni()) }
func (gi) InstanceID() string                                                       { panic(ni()) }
func (gi) IsDevAppServer() bool                                                     { panic(ni()) }
func (gi) ServerSoftware() string                                                   { panic(ni()) }
func (gi) IsCapabilityDisabled(err error) bool                                      { panic(ni()) }
func (gi) IsOverQuota(err error) bool                                               { panic(ni()) }
func (gi) IsTimeoutError(err error) bool                                            { panic(ni()) }

var dummyGIInst = gi{}

// GI returns a dummy GlobalInfo implementation suitable for embedding.
// Every method panics with a message containing the name of the method which
// was unimplemented.
func GI() gae.GlobalInfo { return dummyGIInst }
