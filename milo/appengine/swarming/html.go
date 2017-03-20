// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package swarming

import (
	"errors"
	"net/http"
	"os"

	"golang.org/x/net/context"
	"google.golang.org/api/googleapi"

	"github.com/luci/luci-go/milo/appengine/common"
	"github.com/luci/luci-go/server/router"
	"github.com/luci/luci-go/server/templates"
)

const (
	defaultSwarmingServer    = "chromium-swarm.appspot.com"
	defaultSwarmingDevServer = "chromium-swarm-dev.appspot.com"
)

func getSwarmingHost(r *http.Request) string {
	server := r.FormValue("server")
	switch server {
	case "":
		return defaultSwarmingServer
	case "dev":
		return defaultSwarmingDevServer
	default:
		return server
	}
}

var errUnrecognizedHost = errors.New("Unregistered Swarming Host")

func getSwarmingService(c context.Context, host string) (swarmingService, error) {
	switch host {
	// TODO(hinoka): configure this mapping in luci-config
	case defaultSwarmingServer, defaultSwarmingDevServer,
		"cast-swarming.appspot.com",
		"touch-swarming.appspot.com":
		return newProdService(c, host)

	default:
		return nil, errUnrecognizedHost
	}
}

// Build is for deciphering recipe builds from swarming based off of logs.
type Build struct {
	// bl is the buildLoader to use. A zero value is suitable for production, but
	// this can be overridden for testing.
	bl buildLoader
}

// LogHandler writes the build log to the given response writer.
func LogHandler(c *router.Context) {
	id := c.Params.ByName("id")
	if id == "" {
		common.ErrorPage(c, http.StatusBadRequest, "No id")
		return
	}
	logname := c.Params.ByName("logname")
	if logname == "" {
		common.ErrorPage(c, http.StatusBadRequest, "No log name")
	}

	sf, err := getSwarmingService(c.Context, getSwarmingHost(c.Request))
	if err != nil {
		common.ErrorPage(c, errCode(err), err.Error())
		return
	}

	log, closed, err := swarmingBuildLogImpl(c.Context, sf, id, logname)
	if err != nil {
		common.ErrorPage(c, errCode(err), err.Error())
		return
	}

	templates.MustRender(c.Context, c.Writer, "pages/log.html", templates.Args{
		"Log":    log,
		"Closed": closed,
	})
}

func BuildHandler(c *router.Context) {
	(Build{}).Render(c)
}

// Render renders both the build page and the log.
func (b Build) Render(c *router.Context) {
	// Get the swarming ID
	id := c.Params.ByName("id")
	if id == "" {
		common.ErrorPage(c, http.StatusBadRequest, "No id")
		return
	}

	sf, err := getSwarmingService(c.Context, getSwarmingHost(c.Request))
	if err != nil {
		common.ErrorPage(c, errCode(err), err.Error())
		return
	}

	result, err := b.bl.swarmingBuildImpl(c.Context, sf, c.Request.URL.String(), id)
	if err != nil {
		common.ErrorPage(c, errCode(err), err.Error())
		return
	}

	templates.MustRender(c.Context, c.Writer, "pages/build.html", templates.Args{
		"Build": result,
	})
}

// errCode resolves recognized errors into proper http response codes.
func errCode(err error) int {
	if err == errUnrecognizedHost {
		return http.StatusBadRequest
	}
	if isAPINotFound(err) || os.IsNotExist(err) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

// isAPINotFound returns true if err is a HTTP 404 API response.
func isAPINotFound(err error) bool {
	if apiErr, ok := err.(*googleapi.Error); ok && apiErr.Code == http.StatusNotFound {
		return true
	}
	return false
}
