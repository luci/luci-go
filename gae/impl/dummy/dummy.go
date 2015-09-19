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

func (ds) PutMulti([]*datastore.Key, []datastore.PropertyMap, datastore.PutMultiCB) error {
	panic(ni())
}
func (ds) GetMulti([]*datastore.Key, datastore.MultiMetaGetter, datastore.GetMultiCB) error {
	panic(ni())
}
func (ds) DeleteMulti([]*datastore.Key, datastore.DeleteMultiCB) error { panic(ni()) }
func (ds) NewQuery(string) datastore.Query                             { panic(ni()) }
func (ds) DecodeCursor(string) (datastore.Cursor, error)               { panic(ni()) }
func (ds) Run(*datastore.FinalizedQuery, datastore.RawRunCB) error     { panic(ni()) }
func (ds) RunInTransaction(func(context.Context) error, *datastore.TransactionOptions) error {
	panic(ni())
}
func (ds) Testable() datastore.Testable { return nil }

var dummyDSInst = ds{}

// Datastore returns a dummy datastore.RawInterface implementation suitable
// for embedding. Every method panics with a message containing the name of the
// method which was unimplemented.
func Datastore() datastore.RawInterface { return dummyDSInst }

/////////////////////////////////// mc ////////////////////////////////////

type mc struct{}

func (mc) NewItem(key string) memcache.Item                          { panic(ni()) }
func (mc) AddMulti([]memcache.Item, memcache.RawCB) error            { panic(ni()) }
func (mc) SetMulti([]memcache.Item, memcache.RawCB) error            { panic(ni()) }
func (mc) GetMulti([]string, memcache.RawItemCB) error               { panic(ni()) }
func (mc) DeleteMulti([]string, memcache.RawCB) error                { panic(ni()) }
func (mc) CompareAndSwapMulti([]memcache.Item, memcache.RawCB) error { panic(ni()) }
func (mc) Increment(string, int64, *uint64) (uint64, error)          { panic(ni()) }
func (mc) Flush() error                                              { panic(ni()) }
func (mc) Stats() (*memcache.Statistics, error)                      { panic(ni()) }

var dummyMCInst = mc{}

// Memcache returns a dummy memcache.RawInterface implementation suitable for
// embedding.  Every method panics with a message containing the name of the
// method which was unimplemented.
func Memcache() memcache.RawInterface { return dummyMCInst }

/////////////////////////////////// tq ////////////////////////////////////

type tq struct{}

func (tq) AddMulti([]*taskqueue.Task, string, taskqueue.RawTaskCB) error { panic(ni()) }
func (tq) DeleteMulti([]*taskqueue.Task, string, taskqueue.RawCB) error  { panic(ni()) }
func (tq) Purge(string) error                                            { panic(ni()) }
func (tq) Stats([]string, taskqueue.RawStatsCB) error                    { panic(ni()) }
func (tq) Testable() taskqueue.Testable                                  { return nil }

var dummyTQInst = tq{}

// TaskQueue returns a dummy taskqueue.RawInterface implementation suitable for
// embedding.  Every method panics with a message containing the name of the
// method which was unimplemented.
func TaskQueue() taskqueue.RawInterface { return dummyTQInst }

/////////////////////////////////// i ////////////////////////////////////

type i struct{}

func (i) AccessToken(scopes ...string) (token string, expiry time.Time, err error) { panic(ni()) }
func (i) AppID() string                                                            { return "appid" }
func (i) FullyQualifiedAppID() string                                              { return "dummy~appid" }
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
