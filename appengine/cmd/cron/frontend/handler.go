// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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
	cryptorand "crypto/rand"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"
	"google.golang.org/appengine"

	"github.com/luci/gae/service/info"
	"github.com/luci/gae/service/taskqueue"

	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/middleware"

	"github.com/luci/luci-go/appengine/gaeauth/server"
	"github.com/luci/luci-go/appengine/gaeconfig"
	"github.com/luci/luci-go/appengine/gaemiddleware"

	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/config/impl/memory"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"

	"github.com/luci/luci-go/appengine/cmd/cron/catalog"
	"github.com/luci/luci-go/appengine/cmd/cron/engine"
	"github.com/luci/luci-go/appengine/cmd/cron/task"
	"github.com/luci/luci-go/appengine/cmd/cron/task/buildbucket"
	"github.com/luci/luci-go/appengine/cmd/cron/task/noop"
	"github.com/luci/luci-go/appengine/cmd/cron/task/swarming"
	"github.com/luci/luci-go/appengine/cmd/cron/task/urlfetch"
	"github.com/luci/luci-go/appengine/cmd/cron/ui"
)

//// Global state. See init().

var (
	globalCatalog catalog.Catalog
	globalEngine  engine.Engine

	// Known kinds of tasks.
	managers = []task.Manager{
		&buildbucket.TaskManager{},
		&noop.TaskManager{},
		&swarming.TaskManager{},
		&urlfetch.TaskManager{},
	}
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

// initializeGlobalState does one time initialization for stuff that needs
// active GAE context.
func initializeGlobalState(c context.Context) {
	if info.Get(c).IsDevAppServer() {
		// Dev app server doesn't preserve the state of task queues across restarts,
		// need to reset datastore state accordingly, otherwise everything gets stuck.
		if err := globalEngine.ResetAllJobsOnDevServer(c); err != nil {
			logging.Errorf(c, "Failed to reset jobs: %s", err)
		}
	}
}

// getConfigImpl returns config.Interface implementation to use from the
// catalog.
func getConfigImpl(c context.Context) (config.Interface, error) {
	// Use fake config data on dev server for simplicity.
	if info.Get(c).IsDevAppServer() {
		return memory.New(devServerConfigs()), nil
	}
	return gaeconfig.New(c)
}

// wrap converts the handler to format accepted by middleware lib. It also adds
// context initialization code.
func wrap(h handler) middleware.Handler {
	return func(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		h(&requestContext{c, w, r, p})
	}
}

// base starts middleware chain. It initializes prod context and sets up
// authentication config.
func base(h middleware.Handler) httprouter.Handle {
	methods := auth.Authenticator{
		&server.OAuth2Method{Scopes: []string{server.EmailScope}},
		server.CookieAuth,
		&server.InboundAppIDAuthMethod{},
	}
	wrapper := func(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		globalInit.Do(func() { initializeGlobalState(c) })
		c = auth.SetAuthenticator(c, methods)
		h(c, w, r, p)
	}
	return gaemiddleware.BaseProd(wrapper)
}

// cronHandler returns handler intended for cron jobs.
func cronHandler(h handler) httprouter.Handle {
	return base(gaemiddleware.RequireCron(wrap(h)))
}

// taskQueueHandler returns handler intended for task queue calls.
func taskQueueHandler(name string, h handler) httprouter.Handle {
	return base(gaemiddleware.RequireTaskQueue(name, wrap(h)))
}

//// Routes.

func init() {
	// Dev server likes to restart a lot, and upon a restart math/rand seed is
	// always set to 1, resulting in lots of presumably "random" IDs not being
	// very random. Seed it with real randomness.
	var seed int64
	if err := binary.Read(cryptorand.Reader, binary.LittleEndian, &seed); err != nil {
		panic(err)
	}
	rand.Seed(seed)

	// Setup global singletons.
	globalCatalog = catalog.New(getConfigImpl, "")
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
		PubSubPushPath:       "/pubsub",
	})

	// Setup HTTP routes.
	router := httprouter.New()

	gaemiddleware.InstallHandlers(router, base)
	ui.InstallHandlers(router, base, ui.Config{
		Engine:        globalEngine,
		TemplatesPath: "templates",
	})

	router.GET("/_ah/warmup", base(wrap(warmupHandler)))
	router.GET("/_ah/start", base(wrap(warmupHandler)))
	router.POST("/pubsub", base(wrap(pubsubPushHandler)))
	router.GET("/internal/cron/read-config", cronHandler(readConfigCron))
	router.POST("/internal/tasks/read-project-config", taskQueueHandler("read-project-config", readProjectConfigTask))
	router.POST("/internal/tasks/timers", taskQueueHandler("timers", actionTask))
	router.POST("/internal/tasks/invocations", taskQueueHandler("invocations", actionTask))

	// Devserver can't accept PubSub pushes, so allow manual pulls instead to
	// simplify local development.
	if appengine.IsDevAppServer() {
		router.GET("/pubsub/pull/:ManagerName/:Publisher", base(wrap(pubsubPullHandler)))
	}

	http.DefaultServeMux.Handle("/", router)
}

// warmupHandler warms in-memory caches.
func warmupHandler(rc *requestContext) {
	if err := server.Warmup(rc); err != nil {
		rc.fail(500, "Failed to warmup OpenID: %s", err)
		return
	}
	rc.ok()
}

// pubsubPushHandler handles incoming PubSub messages.
func pubsubPushHandler(rc *requestContext) {
	body, err := ioutil.ReadAll(rc.r.Body)
	if err != nil {
		rc.fail(500, "Failed to read the request: %s", err)
		return
	}
	if err = globalEngine.ProcessPubSubPush(rc, body); err != nil {
		rc.err(err, "Failed to process incoming PubSub push")
		return
	}
	rc.ok()
}

// pubsubPullHandler is called on dev server by developer to pull pubsub
// messages from a topic created for a publisher.
func pubsubPullHandler(rc *requestContext) {
	if !appengine.IsDevAppServer() {
		rc.fail(403, "Not a dev server")
		return
	}
	err := globalEngine.PullPubSubOnDevServer(
		rc, rc.p.ByName("ManagerName"), rc.p.ByName("Publisher"))
	if err != nil {
		rc.err(err, "Failed to pull PubSub messages")
	} else {
		rc.ok()
	}
}

// readConfigCron grabs a list of projects from the catalog and datastore and
// dispatches task queue tasks to update each project's cron jobs.
func readConfigCron(c *requestContext) {
	projectsToVisit := map[string]bool{}

	// Visit all projects in the catalog.
	ctx, _ := context.WithTimeout(c.Context, 150*time.Second)
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
	ctx, _ := context.WithTimeout(c.Context, 150*time.Second)
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
