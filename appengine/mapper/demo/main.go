// Copyright 2018 The LUCI Authors.
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

// Package demo contains minimal demo for 'mapper' package.
package demo

import (
	"context"
	"fmt"
	"net/http"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/appengine/gaemiddleware/standard"
	"go.chromium.org/luci/appengine/mapper"
	"go.chromium.org/luci/appengine/tq"
)

func makeDumpingMapper(c context.Context, j *mapper.Job, shardIdx int) (mapper.Mapper, error) {
	return func(c context.Context, keys []*datastore.Key) error {
		logging.Infof(c, "Got %d keys:", len(keys))
		for _, k := range keys {
			logging.Infof(c, "%s", k)
		}
		return nil
	}, nil
}

// TestEntry would be an entity in the datastore.
type TestEntity struct {
	ID int64 `gae:"$id"`
}

func init() {

	r := router.New()
	base := standard.Base()
	standard.InstallHandlers(r)

	tasks := tq.Dispatcher{}

	mappers := mapper.Controller{}
	mappers.RegisterFactory("dumping/v1", makeDumpingMapper)
	mappers.Install(&tasks)

	tasks.InstallRoutes(r, base)

	// Populate creates a bunch of entities.
	r.GET("/populate", base, func(c *router.Context) {
		ents := make([]TestEntity, 1024)
		if err := datastore.Put(c.Context, ents); err != nil {
			panic(err)
		}
		fmt.Fprintf(c.Writer, "Done")
	})

	// Launch launches the mapper job.
	r.GET("/launch", base, func(c *router.Context) {
		jobID, err := mappers.LaunchJob(c.Context, &mapper.JobConfig{
			Query: mapper.Query{
				Kind: "TestEntity",
			},
			Mapper:     "dumping/v1",
			ShardCount: 8,
			PageSize:   64,
		})
		if err != nil {
			panic(err)
		}
		fmt.Fprintf(c.Writer, "Launched job %d", jobID)
	})

	http.DefaultServeMux.Handle("/", r)
}
