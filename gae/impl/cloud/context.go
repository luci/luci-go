// Copyright 2016 The LUCI Authors.
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
	"fmt"

	"github.com/luci/gae/impl/dummy"
	"github.com/luci/gae/impl/memory"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/mail"
	mc "github.com/luci/gae/service/memcache"
	"github.com/luci/gae/service/module"
	"github.com/luci/gae/service/taskqueue"
	"github.com/luci/gae/service/user"

	"cloud.google.com/go/compute/metadata"
	"cloud.google.com/go/datastore"
	"github.com/bradfitz/gomemcache/memcache"

	"golang.org/x/net/context"
)

// Config is a full-stack cloud service configuration. A user can selectively
// populate its fields, and services for the populated fields will be installed
// in the Context and available.
//
// Because the "impl/cloud" service collection is a composite set of cloud
// services, the user can choose services based on their configuration.
type Config struct {
	// DS is the cloud datastore client. If populated, the datastore service will
	// be installed.
	DS *datastore.Client

	// MC is the memcache service client. If populated, the memcache service will
	// be installed.
	MC *memcache.Client
}

// Use installs the Config into the supplied Context. Services will be installed
// based on the fields that are populated in Config.
//
// Any services that are missing will have "impl/dummy" stubs installed. These
// stubs will panic if called.
func (cfg Config) Use(c context.Context) context.Context {
	// Dummy services that we don't support.
	c = mail.Set(c, dummy.Mail())
	c = module.Set(c, dummy.Module())
	c = taskqueue.SetRaw(c, dummy.TaskQueue())
	c = user.Set(c, dummy.User())

	c = useInfo(c)

	// datastore service
	if cfg.DS != nil {
		cds := cloudDatastore{
			client: cfg.DS,
		}
		c = cds.use(c)
	} else {
		c = ds.SetRaw(c, dummy.Datastore())
	}

	// memcache service
	if cfg.MC != nil {
		mc := memcacheClient{
			client: cfg.MC,
		}
		c = mc.use(c)
	} else {
		c = mc.SetRaw(c, dummy.Memcache())
	}

	return c
}

// UseFlex installs a set of cloud services into the context with services
// supported by AppEngine Flex, including:
// * Info
// * Datastore
func UseFlex(c context.Context) context.Context {
	// Flex is on GCE, so we can get the project ID from the metadata server.
	project, err := metadata.Get("project/project-id")
	if err != nil {
		panic(fmt.Errorf("could not get project ID, not on GCE? %s", err.Error()))
	}
	// Use the memory implementation of Info.
	c = memory.UseInfo(c, project)
	// Create a datastore client.
	client, err := datastore.NewClient(c, project)
	if err != nil {
		panic(err)
	}
	cds := cloudDatastore{
		client: client,
	}
	return cds.use(c)
}
