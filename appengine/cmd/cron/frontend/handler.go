// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package frontend implements GAE web server for luci-cron service.
//
// Due to the way classic GAE imports work, this package can not have
// subpackages (or at least subpackages referenced via absolute import path).
// We can't use relative imports because luci-go will then become unbuildable
// by regular (non GAE) toolset.
//
// See https://groups.google.com/forum/#!topic/google-appengine-go/dNhqV6PBqVc.
package frontend

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"sync"

	"golang.org/x/net/context"

	"github.com/luci/gae/impl/prod"
	"github.com/luci/gae/service/info"
	"github.com/luci/gae/service/taskqueue"

	"github.com/luci/luci-go/appengine/gaeauth"
	"github.com/luci/luci-go/appengine/gaelogger"
	cfgmemory "github.com/luci/luci-go/common/config/impl/memory"
	cfgremote "github.com/luci/luci-go/common/config/impl/remote"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"

	cat "github.com/luci/luci-go/appengine/cmd/cron/catalog"
	eng "github.com/luci/luci-go/appengine/cmd/cron/engine"

	"github.com/luci/luci-go/appengine/cmd/cron/task"
	"github.com/luci/luci-go/appengine/cmd/cron/task/noop"
	"github.com/luci/luci-go/appengine/cmd/cron/task/urlfetch"
)

//// Global state. See init().

var (
	catalog cat.Catalog
	engine  eng.Engine

	// Known kinds of tasks.
	managers = []task.Manager{
		&noop.TaskManager{},
		&urlfetch.TaskManager{},
	}
)

const (
	// configServiceURL is URL of luci-config service.
	// TODO(vadimsh): Make it configurable.
	configServiceURL = "https://luci-config.appspot.com"
)

//// Helpers.

type handler func(c *requestContext)

type requestContext struct {
	context.Context

	w http.ResponseWriter
	r *http.Request
}

// fail writes error message to the log and the response and sets status code.
func (c *requestContext) fail(code int, msg string, args ...interface{}) {
	body := fmt.Sprintf(msg, args...)
	logging.Errorf(c, "HTTP %d: %s", code, body)
	http.Error(c.w, body, code)
}

// err sets status to 500 on transient errors or 202 on fatal ones. Returning
// status code in range [200–299] is the only way to tell Task Queues to stop
// retrying the task.
func (c *requestContext) err(e error, msg string, args ...interface{}) {
	code := 500
	if !errors.IsTransient(e) {
		code = 202
	}
	args = append(args, e)
	c.fail(code, msg+" - %s", args...)
}

// ok sets status to 200 and puts "OK" in response.
func (c *requestContext) ok() {
	c.w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	c.w.WriteHeader(200)
	fmt.Fprintln(c.w, "OK")
}

///

var globalInit sync.Once
var isProdGAE = true

func makeProdRequestContext(w http.ResponseWriter, r *http.Request) *requestContext {
	c := prod.UseRequest(r)
	c = gaelogger.Use(c)
	c = gaeauth.Use(c, nil, nil)

	// Use fake config data on dev server for simplicity.
	if info.Get(c).IsDevAppServer() {
		c = cfgmemory.Use(c, devServerConfigs())
	} else {
		c = cfgremote.Use(c, configServiceURL+"/_ah/api/config/v1/")
	}

	rc := &requestContext{c, w, r}

	// One time initialization for stuff that needs active GAE context.
	globalInit.Do(func() { initializeGlobalState(rc) })
	return rc
}

func initializeGlobalState(rc *requestContext) {
	// Dev app server doesn't preserve the state of task queues across restarts,
	// need to reset datastore state accordingly, otherwise everything gets stuck.
	if info.Get(rc.Context).IsDevAppServer() {
		isProdGAE = false
		if err := engine.ResetAllJobsOnDevServer(rc.Context); err != nil {
			logging.Errorf(rc.Context, "Failed to reset jobs: %s", err)
		}
	}
}

func gaeHandler(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h(makeProdRequestContext(w, r))
	}
}

func cronHandler(h handler) http.HandlerFunc {
	return gaeHandler(func(c *requestContext) {
		if c.r.Header.Get("X-AppEngine-Cron") != "true" && isProdGAE {
			c.fail(403, "Only internal cron jobs can do this")
		} else {
			h(c)
		}
	})
}

func taskQueueHandler(queue string, h handler) http.HandlerFunc {
	return gaeHandler(func(c *requestContext) {
		got := c.r.Header.Get("X-AppEngine-QueueName")
		if got != queue && isProdGAE {
			c.fail(403, "Only internal queue %q can call this, got %q", queue, got)
		} else {
			h(c)
		}
	})
}

//// Routes.

func init() {
	// Setup global singletons.
	catalog = cat.NewCatalog()
	for _, m := range managers {
		if err := catalog.RegisterTaskManager(m); err != nil {
			panic(err)
		}
	}
	engine = eng.NewEngine(eng.Config{
		Catalog:              catalog,
		TimersQueuePath:      "/internal/tasks/timers",
		TimersQueueName:      "timers",
		InvocationsQueuePath: "/internal/tasks/invocations",
		InvocationsQueueName: "invocations",
	})

	// Setup HTTP routes.
	registerFrontendHandlers(http.DefaultServeMux)
	registerBackendHandlers(http.DefaultServeMux)
}

func registerFrontendHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, "Hi there!")
	})
	// To call initializeGlobalState on devserver sooner than later.
	mux.HandleFunc("/_ah/warmup", gaeHandler(func(c *requestContext) { c.ok() }))
}

func registerBackendHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/internal/cron/read-config", cronHandler(readConfigCron))
	mux.HandleFunc(
		"/internal/tasks/read-project-config",
		taskQueueHandler("read-project-config", readProjectConfigTask))
	mux.HandleFunc("/internal/tasks/timers", taskQueueHandler("timers", actionTask))
	mux.HandleFunc("/internal/tasks/invocations", taskQueueHandler("invocations", actionTask))
}

//// Actual handlers.

// readConfigCron grabs a list of projects from the catalog and datastore and
// dispatches task queue tasks to update each project's cron jobs.
func readConfigCron(c *requestContext) {
	projectsToVisit := map[string]bool{}

	// Visit all projects in the catalog.
	projects, err := catalog.GetAllProjects(c.Context)
	if err != nil {
		c.err(err, "Failed to grab a list of project IDs from catalog")
		return
	}
	for _, id := range projects {
		projectsToVisit[id] = true
	}

	// Also visit all registered projects that do not show up in the catalog
	// listing anymore. It will unregister all crons belonging to them.
	existing, err := engine.GetAllProjects(c.Context)
	if err != nil {
		c.err(err, "Failed to grab a list of project IDs from datastore")
		return
	}
	for _, id := range existing {
		projectsToVisit[id] = true
	}

	// Handle each project in its own task to avoid "bad" projects (e.g. ones with
	// lots of crons) to slow down "good" ones.
	tasks := make([]*taskqueue.Task, 0, len(projectsToVisit))
	for projectID := range projectsToVisit {
		tasks = append(tasks, &taskqueue.Task{
			Path: "/internal/tasks/read-project-config?projectID=" + url.QueryEscape(projectID),
		})
	}
	tq := taskqueue.Get(c)
	if err = tq.AddMulti(tasks, "read-project-config"); err != nil {
		c.err(errors.WrapTransient(err), "Failed to add tasks to task queue")
	} else {
		c.ok()
	}
}

// readProjectConfigTask grabs a list of cron jobs in a project from catalog,
// updates all changed cron jobs, adds new ones, disables old ones.
func readProjectConfigTask(c *requestContext) {
	projectID := c.r.URL.Query().Get("projectID")
	if projectID == "" {
		// Return 202 to avoid retry, it is fatal error.
		c.fail(202, "Missing projectID query attribute")
		return
	}
	jobs, err := catalog.GetProjectJobs(c.Context, projectID)
	if err != nil {
		c.err(err, "Failed to query for a list of jobs")
		return
	}
	if err = engine.UpdateProjectJobs(c.Context, projectID, jobs); err != nil {
		c.err(err, "Failed to update some cron jobs")
		return
	}
	c.ok()
}

// actionTask is used to route actions emitted by cron job state transitions
// back into Engine (see enqueueActions).
func actionTask(c *requestContext) {
	body, err := ioutil.ReadAll(c.r.Body)
	if err != nil {
		c.fail(500, "Failed to read request body: %s", err)
		return
	}
	count, _ := strconv.Atoi(c.r.Header.Get("X-AppEngine-TaskExecutionCount"))
	err = engine.ExecuteSerializedAction(c.Context, body, count)
	if err != nil {
		c.err(err, "Error when executing the action")
		return
	}
	c.ok()
}
