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

package dummy

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"
	"go.chromium.org/gae/service/mail"
	"go.chromium.org/gae/service/memcache"
	"go.chromium.org/gae/service/module"
	"go.chromium.org/gae/service/taskqueue"
	"go.chromium.org/gae/service/user"

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
				case "i":
					iface = "Info"
				case "m":
					iface = "Mail"
				case "mc":
					iface = "Memcache"
				case "mod":
					iface = "Module"
				case "tq":
					iface = "TaskQueue"
				case "u":
					iface = "User"
				}
				funcName = parts[len(parts)-1]
			}
		}
	}

	return fmt.Errorf(niFmtStr, iface, funcName)
}

/////////////////////////////////// ds ////////////////////////////////////

type ds struct{}

func (ds) AllocateIDs([]*datastore.Key, datastore.NewKeyCB) error { panic(ni()) }
func (ds) PutMulti([]*datastore.Key, []datastore.PropertyMap, datastore.NewKeyCB) error {
	panic(ni())
}
func (ds) GetMulti([]*datastore.Key, datastore.MultiMetaGetter, datastore.GetMultiCB) error {
	panic(ni())
}
func (ds) DeleteMulti([]*datastore.Key, datastore.DeleteMultiCB) error { panic(ni()) }
func (ds) DecodeCursor(string) (datastore.Cursor, error)               { panic(ni()) }
func (ds) Count(*datastore.FinalizedQuery) (int64, error)              { panic(ni()) }
func (ds) Run(*datastore.FinalizedQuery, datastore.RawRunCB) error     { panic(ni()) }
func (ds) RunInTransaction(func(context.Context) error, *datastore.TransactionOptions) error {
	panic(ni())
}
func (ds) WithoutTransaction() context.Context       { panic(ni()) }
func (ds) CurrentTransaction() datastore.Transaction { panic(ni()) }

func (ds) Constraints() datastore.Constraints { return datastore.Constraints{} }
func (ds) GetTestable() datastore.Testable    { return nil }

var dummyDSInst = ds{}

// Datastore returns a dummy datastore.RawInterface implementation suitable
// for embedding. Every method panics with a message containing the name of the
// method which was unimplemented.
func Datastore() datastore.RawInterface { return dummyDSInst }

/////////////////////////////////// mc ////////////////////////////////////

type mc struct{}

func (mc) NewItem(string) memcache.Item                              { panic(ni()) }
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

func (tq) AddMulti([]*taskqueue.Task, string, taskqueue.RawTaskCB) error            { panic(ni()) }
func (tq) DeleteMulti([]*taskqueue.Task, string, taskqueue.RawCB) error             { panic(ni()) }
func (tq) Lease(int, string, time.Duration) ([]*taskqueue.Task, error)              { panic(ni()) }
func (tq) LeaseByTag(int, string, time.Duration, string) ([]*taskqueue.Task, error) { panic(ni()) }
func (tq) ModifyLease(*taskqueue.Task, string, time.Duration) error                 { panic(ni()) }
func (tq) Purge(string) error                                                       { panic(ni()) }
func (tq) Stats([]string, taskqueue.RawStatsCB) error                               { panic(ni()) }
func (tq) Constraints() taskqueue.Constraints                                       { panic(ni()) }
func (tq) GetTestable() taskqueue.Testable                                          { return nil }

var dummyTQInst = tq{}

// TaskQueue returns a dummy taskqueue.RawInterface implementation suitable for
// embedding.  Every method panics with a message containing the name of the
// method which was unimplemented.
func TaskQueue() taskqueue.RawInterface { return dummyTQInst }

/////////////////////////////////// i ////////////////////////////////////

type i struct{}

func (i) AccessToken(...string) (token string, expiry time.Time, err error) {
	panic(ni())
}
func (i) AppID() string               { return "appid" }
func (i) FullyQualifiedAppID() string { return "dummy~appid" }
func (i) GetNamespace() string        { return "dummy-namespace" }
func (i) ModuleHostname(module, version, instance string) (string, error) {
	if instance != "" {
		panic(ni())
	}
	if module == "" {
		module = "module"
	}
	if version == "" {
		version = "version"
	}
	return fmt.Sprintf("%s.%s.dummy-appid.example.com", version, module), nil
}
func (i) ModuleName() string                                             { return "module" }
func (i) DefaultVersionHostname() string                                 { return "dummy-appid.example.com" }
func (i) PublicCertificates() ([]info.Certificate, error)                { panic(ni()) }
func (i) RequestID() string                                              { panic(ni()) }
func (i) ServiceAccount() (string, error)                                { panic(ni()) }
func (i) SignBytes([]byte) (keyName string, signature []byte, err error) { panic(ni()) }
func (i) VersionID() string                                              { panic(ni()) }
func (i) Namespace(string) (context.Context, error)                      { panic(ni()) }
func (i) Datacenter() string                                             { panic(ni()) }
func (i) InstanceID() string                                             { panic(ni()) }
func (i) IsDevAppServer() bool                                           { panic(ni()) }
func (i) ServerSoftware() string                                         { panic(ni()) }
func (i) IsCapabilityDisabled(error) bool                                { panic(ni()) }
func (i) IsOverQuota(error) bool                                         { panic(ni()) }
func (i) IsTimeoutError(error) bool                                      { panic(ni()) }
func (i) GetTestable() info.Testable                                     { panic(ni()) }

var dummyInfoInst = i{}

// Info returns a dummy info.RawInterface implementation suitable for embedding.
// Every method panics with a message containing the name of the method which
// was unimplemented.
func Info() info.RawInterface { return dummyInfoInst }

////////////////////////////////////// u ///////////////////////////////////////

type u struct{}

func (u) Current() *user.User                              { panic(ni()) }
func (u) CurrentOAuth(...string) (*user.User, error)       { panic(ni()) }
func (u) IsAdmin() bool                                    { panic(ni()) }
func (u) LoginURL(string) (string, error)                  { panic(ni()) }
func (u) LoginURLFederated(string, string) (string, error) { panic(ni()) }
func (u) LogoutURL(string) (string, error)                 { panic(ni()) }
func (u) OAuthConsumerKey() (string, error)                { panic(ni()) }
func (u) GetTestable() user.Testable                       { panic(ni()) }

var dummyUserInst = u{}

// User returns a dummy user.Interface implementation suitable for embedding.
// Every method panics with a message containing the name of the method which
// was unimplemented.
func User() user.RawInterface { return dummyUserInst }

////////////////////////////////////// m ///////////////////////////////////////

type m struct{}

func (m) Send(*mail.Message) error         { panic(ni()) }
func (m) SendToAdmins(*mail.Message) error { panic(ni()) }
func (m) GetTestable() mail.Testable       { panic(ni()) }

var dummyMailInst = m{}

// Mail returns a dummy mail.Interface implementation suitable for embedding.
// Every method panics with a message containing the name of the method which
// was unimplemented.
func Mail() mail.RawInterface { return dummyMailInst }

/////////////////////////////////// mod ////////////////////////////////////

type mod struct{}

func (mod) List() ([]string, error)                          { panic(ni()) }
func (mod) NumInstances(module, version string) (int, error) { panic(ni()) }
func (mod) SetNumInstances(module, version string, instances int) error {
	panic(ni())
}
func (mod) Versions(module string) ([]string, error)     { panic(ni()) }
func (mod) DefaultVersion(module string) (string, error) { panic(ni()) }
func (mod) Start(module, version string) error           { panic(ni()) }
func (mod) Stop(module, version string) error            { panic(ni()) }

var dummyModuleInst = mod{}

// Module returns a dummy module.Interface implementation suitable for
// embedding. Every method panics with a message containing the name of the
// method which was unimplemented.
func Module() module.RawInterface { return dummyModuleInst }
