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
	"time"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/gae/service/info"
	"github.com/luci/gae/service/taskqueue"

	"github.com/luci/luci-go/appengine/gaeauth"
	"github.com/luci/luci-go/appengine/middleware"

	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/config/impl/memory"
	"github.com/luci/luci-go/common/config/impl/remote"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"

	"github.com/luci/luci-go/appengine/cmd/cron/catalog"
	"github.com/luci/luci-go/appengine/cmd/cron/engine"
	"github.com/luci/luci-go/appengine/cmd/cron/task"
	"github.com/luci/luci-go/appengine/cmd/cron/task/noop"
	"github.com/luci/luci-go/appengine/cmd/cron/task/urlfetch"
)

//// Global state. See init().

var (
	globalCatalog catalog.Catalog
	globalEngine  engine.Engine

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

	// configServiceTimeout is deadline for luci-config url fetch calls.
	configServiceTimeout = 150 * time.Second
)

//// Helpers.

type handler func(c *requestContext)

type requestContext struct {
	context.Context

	w http.ResponseWriter
	r *http.Request
	p httprouter.Params
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

// initializeGlobalState does one time initialization for stuff that needs
// active GAE context.
func initializeGlobalState(rc *requestContext) {
	// Dev app server doesn't preserve the state of task queues across restarts,
	// need to reset datastore state accordingly, otherwise everything gets stuck.
	if info.Get(rc.Context).IsDevAppServer() {
		isProdGAE = false
		if err := globalEngine.ResetAllJobsOnDevServer(rc.Context); err != nil {
			logging.Errorf(rc.Context, "Failed to reset jobs: %s", err)
		}
	}
}

// getConfigImpl returns config.Interface implementation to use from the
// catalog.
func getConfigImpl(c context.Context) config.Interface {
	// Use fake config data on dev server for simplicity.
	if info.Get(c).IsDevAppServer() {
		return memory.New(devServerConfigs())
	}
	return remote.New(c, configServiceURL+"/_ah/api/config/v1/")
}

// wrap converts the handler to format accepted by middleware lib. It also adds
// context initialization code.
func wrap(h handler) middleware.Handler {
	return func(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		rc := &requestContext{gaeauth.Use(c, nil, nil), w, r, p}
		globalInit.Do(func() { initializeGlobalState(rc) })
		h(rc)
	}
}

// publicHandler returns handler for publicly accessible routes.
func publicHandler(h handler) httprouter.Handle {
	return middleware.BaseProd(wrap(h))
}

// cronHandler returns handler intended for cron jobs.
func cronHandler(h handler) httprouter.Handle {
	return middleware.BaseProd(middleware.RequireCron(wrap(h)))
}

// taskQueueHandler returns handler intended for task queue calls.
func taskQueueHandler(name string, h handler) httprouter.Handle {
	return middleware.BaseProd(middleware.RequireTaskQueue(name, wrap(h)))
}

//// Routes.

func init() {
	// Setup global singletons.
	globalCatalog = catalog.New(getConfigImpl)
	for _, m := range managers {
		if err := globalCatalog.RegisterTaskManager(m); err != nil {
			panic(err)
		}
	}
	globalEngine = engine.NewEngine(engine.Config{
		Catalog:              globalCatalog,
		TimersQueuePath:      "/internal/tasks/timers",
		TimersQueueName:      "timers",
		InvocationsQueuePath: "/internal/tasks/invocations",
		InvocationsQueueName: "invocations",
	})

	// Setup HTTP routes.
	router := httprouter.New()
	registerFrontendHandlers(router)
	registerBackendHandlers(router)
	http.DefaultServeMux.Handle("/", router)
}

func registerFrontendHandlers(router *httprouter.Router) {
	router.GET("/", publicHandler(indexPage))
	router.GET("/_ah/warmup", publicHandler(warmupHandler))
}

func registerBackendHandlers(router *httprouter.Router) {
	router.GET("/internal/cron/read-config", cronHandler(readConfigCron))
	router.POST("/internal/tasks/read-project-config", taskQueueHandler("read-project-config", readProjectConfigTask))
	router.POST("/internal/tasks/timers", taskQueueHandler("timers", actionTask))
	router.POST("/internal/tasks/invocations", taskQueueHandler("invocations", actionTask))
}

//// Frontend handlers.

func indexPage(rc *requestContext) {
	fmt.Fprint(rc.w, "Hi there!")
}

func warmupHandler(rc *requestContext) {
	rc.ok()
}

//// Backend handlers.

// readConfigCron grabs a list of projects from the catalog and datastore and
// dispatches task queue tasks to update each project's cron jobs.
func readConfigCron(c *requestContext) {
	projectsToVisit := map[string]bool{}

	// Visit all projects in the catalog.
	ctx, _ := context.WithTimeout(c.Context, configServiceTimeout)
	projects, err := globalCatalog.GetAllProjects(ctx)
	if err != nil {
		c.err(err, "Failed to grab a list of project IDs from catalog")
		return
	}
	for _, id := range projects {
		projectsToVisit[id] = true
	}

	// Also visit all registered projects that do not show up in the catalog
	// listing anymore. It will unregister all crons belonging to them.
	existing, err := globalEngine.GetAllProjects(c.Context)
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
	ctx, _ := context.WithTimeout(c.Context, configServiceTimeout)
	jobs, err := globalCatalog.GetProjectJobs(ctx, projectID)
	if err != nil {
		c.err(err, "Failed to query for a list of jobs")
		return
	}
	if err = globalEngine.UpdateProjectJobs(c.Context, projectID, jobs); err != nil {
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
	err = globalEngine.ExecuteSerializedAction(c.Context, body, count)
	if err != nil {
		c.err(err, "Error when executing the action")
		return
	}
	c.ok()
}
