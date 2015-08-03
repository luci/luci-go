// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dummy

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	"github.com/luci/gae/service/memcache"
	"github.com/luci/gae/service/taskqueue"
	"golang.org/x/net/context"
)

const niFmtStr = "dummy: method %s.%s is not implemented"

// ni returns an error whose message is an appropriate expansion of niFmtStr.
//
// It walks the stack to find out what interface and method it's being
// called from. For example, it might return a message which looks like:
//   dummy: method Datastore.Get is not implemented
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
			if len(parts) > 2 {
				switch parts[len(parts)-2] {
				case "ds":
					iface = "Datastore"
				case "mc":
					iface = "Memcache"
				case "tq":
					iface = "TaskQueue"
				case "i":
					iface = "Info"
				}
				funcName = parts[len(parts)-1]
			}
		}
	}

	return fmt.Errorf(niFmtStr, iface, funcName)
}

/////////////////////////////////// ds ////////////////////////////////////

type ds struct{}

func (ds) NewKey(kind string, sid string, iid int64, par datastore.Key) datastore.Key {
	return datastore.NewKey("dummy~appid", "", kind, sid, iid, par)
}
func (ds) DecodeKey(string) (datastore.Key, error) { panic(ni()) }
func (ds) PutMulti([]datastore.Key, []datastore.PropertyLoadSaver, datastore.PutMultiCB) error {
	panic(ni())
}
func (ds) GetMulti([]datastore.Key, datastore.GetMultiCB) error       { panic(ni()) }
func (ds) DeleteMulti([]datastore.Key, datastore.DeleteMultiCB) error { panic(ni()) }
func (ds) NewQuery(string) datastore.Query                            { panic(ni()) }
func (ds) Run(datastore.Query, datastore.RunCB) error                 { panic(ni()) }
func (ds) RunInTransaction(func(context.Context) error, *datastore.TransactionOptions) error {
	panic(ni())
}

var dummyDSInst = ds{}

// Datastore returns a dummy datastore.Interface implementation suitable
// for embedding. Every method panics with a message containing the name of the
// method which was unimplemented.
func Datastore() datastore.Interface { return dummyDSInst }

/////////////////////////////////// mc ////////////////////////////////////

type mc struct{}

func (mc) Add(memcache.Item) error                             { panic(ni()) }
func (mc) NewItem(key string) memcache.Item                    { panic(ni()) }
func (mc) Set(memcache.Item) error                             { panic(ni()) }
func (mc) Get(string) (memcache.Item, error)                   { panic(ni()) }
func (mc) Delete(string) error                                 { panic(ni()) }
func (mc) CompareAndSwap(memcache.Item) error                  { panic(ni()) }
func (mc) AddMulti([]memcache.Item) error                      { panic(ni()) }
func (mc) SetMulti([]memcache.Item) error                      { panic(ni()) }
func (mc) GetMulti([]string) (map[string]memcache.Item, error) { panic(ni()) }
func (mc) DeleteMulti([]string) error                          { panic(ni()) }
func (mc) CompareAndSwapMulti([]memcache.Item) error           { panic(ni()) }
func (mc) Increment(string, int64, uint64) (uint64, error)     { panic(ni()) }
func (mc) IncrementExisting(string, int64) (uint64, error)     { panic(ni()) }
func (mc) Flush() error                                        { panic(ni()) }
func (mc) Stats() (*memcache.Statistics, error)                { panic(ni()) }

var dummyMCInst = mc{}

// Memcache returns a dummy memcache.Interface implementation suitable for
// embedding.  Every method panics with a message containing the name of the
// method which was unimplemented.
func Memcache() memcache.Interface { return dummyMCInst }

/////////////////////////////////// tq ////////////////////////////////////

type tq struct{}

func (tq) Add(*taskqueue.Task, string) (*taskqueue.Task, error)           { panic(ni()) }
func (tq) Delete(*taskqueue.Task, string) error                           { panic(ni()) }
func (tq) AddMulti([]*taskqueue.Task, string) ([]*taskqueue.Task, error)  { panic(ni()) }
func (tq) DeleteMulti([]*taskqueue.Task, string) error                    { panic(ni()) }
func (tq) Lease(int, string, int) ([]*taskqueue.Task, error)              { panic(ni()) }
func (tq) LeaseByTag(int, string, int, string) ([]*taskqueue.Task, error) { panic(ni()) }
func (tq) ModifyLease(*taskqueue.Task, string, int) error                 { panic(ni()) }
func (tq) Purge(string) error                                             { panic(ni()) }
func (tq) QueueStats([]string) ([]taskqueue.Statistics, error)            { panic(ni()) }

var dummyTQInst = tq{}

// TaskQueue returns a dummy taskqueue.Interface implementation suitable for
// embedding.  Every method panics with a message containing the name of the
// method which was unimplemented.
func TaskQueue() taskqueue.Interface { return dummyTQInst }

/////////////////////////////////// i ////////////////////////////////////

type i struct{}

func (i) AccessToken(scopes ...string) (token string, expiry time.Time, err error) { panic(ni()) }
func (i) AppID() string                                                            { return "dummy~appid" }
func (i) GetNamespace() string                                                     { return "dummy-namespace" }
func (i) ModuleHostname(module, version, instance string) (string, error)          { panic(ni()) }
func (i) ModuleName() string                                                       { panic(ni()) }
func (i) DefaultVersionHostname() string                                           { panic(ni()) }
func (i) PublicCertificates() ([]info.Certificate, error)                          { panic(ni()) }
func (i) RequestID() string                                                        { panic(ni()) }
func (i) ServiceAccount() (string, error)                                          { panic(ni()) }
func (i) SignBytes(bytes []byte) (keyName string, signature []byte, err error)     { panic(ni()) }
func (i) VersionID() string                                                        { panic(ni()) }
func (i) Namespace(namespace string) (context.Context, error)                      { panic(ni()) }
func (i) Datacenter() string                                                       { panic(ni()) }
func (i) InstanceID() string                                                       { panic(ni()) }
func (i) IsDevAppServer() bool                                                     { panic(ni()) }
func (i) ServerSoftware() string                                                   { panic(ni()) }
func (i) IsCapabilityDisabled(err error) bool                                      { panic(ni()) }
func (i) IsOverQuota(err error) bool                                               { panic(ni()) }
func (i) IsTimeoutError(err error) bool                                            { panic(ni()) }

var dummyInfoInst = i{}

// Info returns a dummy info.Interface implementation suitable for embedding.
// Every method panics with a message containing the name of the method which
// was unimplemented.
func Info() info.Interface { return dummyInfoInst }
