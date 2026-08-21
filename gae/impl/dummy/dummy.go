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
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/gae/service/mail"
	"go.chromium.org/luci/gae/service/memcache"
	"go.chromium.org/luci/gae/service/module"
	"go.chromium.org/luci/gae/service/taskqueue"
	"go.chromium.org/luci/gae/service/user"
)

const niFmtStr = "dummy: method %s.%s is not implemented"

// ni returns an error whose message is an appropriate expansion of niFmtStr.
//
// It walks the stack to find out what interface and method it's being
// called from. For example, it might return a message which looks like:
//
//	dummy: method Datastore.Get is not implemented
//
// This allows the various dummy objects below to have clear boilerplate which
// avoids copy+paste errors (such as if each one of them filled in the template
// manually).
//
// If this function is somehow called from something other than one of the
// dummy objects in this package, it will substitute the string UNKNOWN for the
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
				case "Datastore":
					iface = "Datastore"
				case "Info":
					iface = "Info"
				case "Mail":
					iface = "Mail"
				case "Memcache":
					iface = "Memcache"
				case "Module":
					iface = "Module"
				case "TaskQueue":
					iface = "TaskQueue"
				case "User":
					iface = "User"
				}
				funcName = parts[len(parts)-1]
			}
		}
	}

	return fmt.Errorf(niFmtStr, iface, funcName)
}

/////////////////////////////////// Datastore ////////////////////////////////////

// Datastore is a dummy datastore.RawInterface implementation suitable
// for embedding. Every method panics with a message containing the name of the
// method which was unimplemented.
type Datastore struct{}

var _ datastore.RawInterface = Datastore{}

func (Datastore) AllocateIDs([]*datastore.Key, datastore.NewKeyCB) error { panic(ni()) }
func (Datastore) PutMulti([]*datastore.Key, []datastore.PropertyMap, datastore.NewKeyCB) error {
	panic(ni())
}
func (Datastore) GetMulti([]*datastore.Key, datastore.MultiMetaGetter, datastore.GetMultiCB) error {
	panic(ni())
}
func (Datastore) DeleteMulti([]*datastore.Key, datastore.DeleteMultiCB) error { panic(ni()) }
func (Datastore) DecodeCursor(string) (datastore.RawCursor, error)            { panic(ni()) }
func (Datastore) Count(*datastore.FinalizedQuery) (int64, error)              { panic(ni()) }
func (Datastore) RunQuery(*datastore.FinalizedQuery) datastore.RawQueryIter   { panic(ni()) }
func (Datastore) RunInTransaction(func(context.Context) error, *datastore.TransactionOptions) error {
	panic(ni())
}
func (Datastore) WithoutTransaction() context.Context       { panic(ni()) }
func (Datastore) CurrentTransaction() datastore.Transaction { panic(ni()) }

func (Datastore) Constraints() datastore.Constraints { return datastore.Constraints{} }
func (Datastore) GetTestable() datastore.Testable    { return nil }

/////////////////////////////////// Memcache ////////////////////////////////////

// Memcache is a dummy memcache.RawInterface implementation suitable for
// embedding.  Every method panics with a message containing the name of the
// method which was unimplemented.
type Memcache struct{}

var _ memcache.RawInterface = Memcache{}

func (Memcache) NewItem(string) memcache.Item                              { panic(ni()) }
func (Memcache) AddMulti([]memcache.Item, memcache.RawCB) error            { panic(ni()) }
func (Memcache) SetMulti([]memcache.Item, memcache.RawCB) error            { panic(ni()) }
func (Memcache) GetMulti([]string, memcache.RawItemCB) error               { panic(ni()) }
func (Memcache) DeleteMulti([]string, memcache.RawCB) error                { panic(ni()) }
func (Memcache) CompareAndSwapMulti([]memcache.Item, memcache.RawCB) error { panic(ni()) }
func (Memcache) Increment(string, int64, *uint64) (uint64, error)          { panic(ni()) }
func (Memcache) Flush() error                                              { panic(ni()) }
func (Memcache) Stats() (*memcache.Statistics, error)                      { panic(ni()) }

/////////////////////////////////// TaskQueue ////////////////////////////////////

// TaskQueue is a dummy taskqueue.RawInterface implementation suitable for
// embedding.  Every method panics with a message containing the name of the
// method which was unimplemented.
type TaskQueue struct{}

var _ taskqueue.RawInterface = TaskQueue{}

func (TaskQueue) AddMulti([]*taskqueue.Task, string, taskqueue.RawTaskCB) error { panic(ni()) }
func (TaskQueue) DeleteMulti([]*taskqueue.Task, string, taskqueue.RawCB) error  { panic(ni()) }
func (TaskQueue) Lease(int, string, time.Duration) ([]*taskqueue.Task, error)   { panic(ni()) }
func (TaskQueue) LeaseByTag(int, string, time.Duration, string) ([]*taskqueue.Task, error) {
	panic(ni())
}
func (TaskQueue) ModifyLease(*taskqueue.Task, string, time.Duration) error { panic(ni()) }
func (TaskQueue) Purge(string) error                                       { panic(ni()) }
func (TaskQueue) Stats([]string, taskqueue.RawStatsCB) error               { panic(ni()) }
func (TaskQueue) Constraints() taskqueue.Constraints                       { panic(ni()) }
func (TaskQueue) GetTestable() taskqueue.Testable                          { return nil }

/////////////////////////////////// Info ////////////////////////////////////

// Info is a dummy info.RawInterface implementation suitable for embedding.
// Every method panics with a message containing the name of the method which
// was unimplemented.
type Info struct{}

var _ info.RawInterface = Info{}

func (Info) AccessToken(...string) (token string, expiry time.Time, err error) {
	panic(ni())
}
func (Info) AppID() string               { return "appid" }
func (Info) FullyQualifiedAppID() string { return "dummy~appid" }
func (Info) GetNamespace() string        { return "dummy-namespace" }
func (Info) ModuleHostname(module, version, instance string) (string, error) {
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
func (Info) ModuleName() string                                             { return "module" }
func (Info) DefaultVersionHostname() string                                 { return "dummy-appid.example.com" }
func (Info) PublicCertificates() ([]info.Certificate, error)                { panic(ni()) }
func (Info) RequestID() string                                              { panic(ni()) }
func (Info) ServiceAccount() (string, error)                                { panic(ni()) }
func (Info) SignBytes([]byte) (keyName string, signature []byte, err error) { panic(ni()) }
func (Info) VersionID() string                                              { panic(ni()) }
func (Info) Namespace(string) (context.Context, error)                      { panic(ni()) }
func (Info) Datacenter() string                                             { panic(ni()) }
func (Info) InstanceID() string                                             { panic(ni()) }
func (Info) IsDevAppServer() bool                                           { panic(ni()) }
func (Info) ServerSoftware() string                                         { panic(ni()) }
func (Info) IsCapabilityDisabled(error) bool                                { panic(ni()) }
func (Info) IsOverQuota(error) bool                                         { panic(ni()) }
func (Info) IsTimeoutError(error) bool                                      { panic(ni()) }
func (Info) GetTestable() info.Testable                                     { panic(ni()) }

////////////////////////////////////// User ///////////////////////////////////////

// User is a dummy user.RawInterface implementation suitable for embedding.
// Every method panics with a message containing the name of the method which
// was unimplemented.
type User struct{}

var _ user.RawInterface = User{}

func (User) Current() *user.User                              { panic(ni()) }
func (User) CurrentOAuth(...string) (*user.User, error)       { panic(ni()) }
func (User) IsAdmin() bool                                    { panic(ni()) }
func (User) LoginURL(string) (string, error)                  { panic(ni()) }
func (User) LoginURLFederated(string, string) (string, error) { panic(ni()) }
func (User) LogoutURL(string) (string, error)                 { panic(ni()) }
func (User) OAuthConsumerKey() (string, error)                { panic(ni()) }
func (User) GetTestable() user.Testable                       { panic(ni()) }

////////////////////////////////////// Mail ///////////////////////////////////////

// Mail is a dummy mail.Interface implementation suitable for embedding.
// Every method panics with a message containing the name of the method which
// was unimplemented.
type Mail struct{}

var _ mail.RawInterface = Mail{}

func (Mail) Send(*mail.Message) error         { panic(ni()) }
func (Mail) SendToAdmins(*mail.Message) error { panic(ni()) }
func (Mail) GetTestable() mail.Testable       { panic(ni()) }

/////////////////////////////////// Module ////////////////////////////////////

// Module is a dummy module.RawInterface implementation suitable for embedding.
// Every method panics with a message containing the name of the method which
// was unimplemented.
type Module struct{}

var _ module.RawInterface = Module{}

func (Module) List() ([]string, error)                          { panic(ni()) }
func (Module) NumInstances(module, version string) (int, error) { panic(ni()) }
func (Module) SetNumInstances(module, version string, instances int) error {
	panic(ni())
}
func (Module) Versions(module string) ([]string, error)     { panic(ni()) }
func (Module) DefaultVersion(module string) (string, error) { panic(ni()) }
func (Module) Start(module, version string) error           { panic(ni()) }
func (Module) Stop(module, version string) error            { panic(ni()) }
