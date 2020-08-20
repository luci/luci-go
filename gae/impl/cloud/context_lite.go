// Copyright 2019 The LUCI Authors.
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

package cloud

import (
	"cloud.google.com/go/datastore"
	"github.com/bradfitz/gomemcache/memcache"
	"golang.org/x/net/context"

	"go.chromium.org/gae/impl/dummy"
	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/mail"
	mc "go.chromium.org/gae/service/memcache"
	"go.chromium.org/gae/service/module"
	"go.chromium.org/gae/service/taskqueue"
	"go.chromium.org/gae/service/user"
)

// ConfigLite can be used to configure a context to have supported Cloud
// Services clients and ONLY them.
//
// Currently supports only datastore and its required dependencies.
//
// Unlike Config, it doesn't try to setup logging, intercept HTTP requests,
// provide auth, etc.
type ConfigLite struct {
	// IsDev is true if this is a development execution.
	IsDev bool

	// ProjectID, if not empty, is the project ID returned by the "info" service.
	//
	// If empty, the service will treat requests for this field as not
	// implemented.
	ProjectID string

	// ServiceName, if not empty, is the service (module) name returned by the
	// "info" service.
	//
	// If empty, the service will treat requests for this field as not
	// implemented.
	ServiceName string

	// VersionName, if not empty, is the version name returned by the "info"
	// service.
	//
	// If empty, the service will treat requests for this field as not
	// implemented.
	VersionName string

	// InstanceID, if not empty, is the instance ID returned by the "info"
	// service.
	//
	// If empty, the service will treat requests for this field as not
	// implemented.
	InstanceID string

	// RequestID, if not empty, is the request ID returned by the "info"
	// service.
	//
	// If empty, the service will treat requests for this field as not
	// implemented.
	RequestID string

	// ServiceAccountName, if not empty, is the service account name returned by
	// the "info" service.
	//
	// If empty, the service will treat requests for this field as not
	// implemented.
	ServiceAccountName string

	// DS is the Cloud Datastore client.
	//
	// If populated, the datastore service will be installed.
	DS *datastore.Client

	// MC is the memcache service client.
	//
	// If populated, the memcache service will be installed.
	MC *memcache.Client
}

// Use configures the context with implementation of Cloud Services.
//
// Any services that are missing will have "impl/dummy" stubs installed. These
// stubs will panic if called.
func (cfg *ConfigLite) Use(c context.Context) context.Context {
	// Dummy services that we don't support.
	c = mail.Set(c, dummy.Mail())
	c = module.Set(c, dummy.Module())
	c = taskqueue.SetRaw(c, dummy.TaskQueue())
	c = user.Set(c, dummy.User())

	c = useInfo(c, &serviceInstanceGlobalInfo{
		IsDev:              cfg.IsDev,
		ProjectID:          cfg.ProjectID,
		ServiceName:        cfg.ServiceName,
		VersionName:        cfg.VersionName,
		InstanceID:         cfg.InstanceID,
		RequestID:          cfg.RequestID,
		ServiceAccountName: cfg.ServiceAccountName,
	})

	if cfg.DS != nil {
		cds := cloudDatastore{client: cfg.DS}
		c = cds.use(c)
	} else {
		c = ds.SetRaw(c, dummy.Datastore())
	}

	if cfg.MC != nil {
		mc := memcacheClient{client: cfg.MC}
		c = mc.use(c)
	} else {
		c = mc.SetRaw(c, dummy.Memcache())
	}

	return c
}
