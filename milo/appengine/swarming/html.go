// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package swarming

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"golang.org/x/net/context"
	"google.golang.org/api/googleapi"

	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/milo/appengine/common"
	"github.com/luci/luci-go/server/router"
	"github.com/luci/luci-go/server/templates"
)

var errUnrecognizedHost = errors.New("Unregistered Swarming Host")

// getSwarmingHost parses the swarming hostname out of the context.  If
// none is specified, get the default swarming host out of the global
// configs.
func getSwarmingHost(c context.Context, r *http.Request) (string, error) {
	settings := common.GetSettings(c)
	if settings.Swarming == nil {
		err := errors.New("swarming not in settings")
		logging.Errorf(c, err.Error())
		return "", err
	}
	server := r.FormValue("server")
	// If server isn't specified, return the default host.
	if server == "" || server == settings.Swarming.DefaultHost {
		return settings.Swarming.DefaultHost, nil
	}
	// If it is specified, validate the hostname.
	for _, hostname := range settings.Swarming.AllowedHosts {
		if server == hostname {
			return server, nil
		}
	}
	return "", errUnrecognizedHost
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
		common.ErrorPage(c, http.StatusBadRequest, "no id")
		return
	}
	logname := c.Params.ByName("logname")
	if logname == "" {
		common.ErrorPage(c, http.StatusBadRequest, "no log name")
		return
	}

	hostname, err := getSwarmingHost(c.Context, c.Request)
	if err != nil {
		common.ErrorPage(c, http.StatusBadRequest,
			fmt.Sprintf("no swarming host: %s", err.Error()))
		return
	}
	sf, err := newProdService(c.Context, hostname)
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
		common.ErrorPage(c, http.StatusBadRequest, "no id")
		return
	}

	hostname, err := getSwarmingHost(c.Context, c.Request)
	if err != nil {
		common.ErrorPage(c, http.StatusBadRequest,
			fmt.Sprintf("no swarming host: %s", err.Error()))
		return
	}
	sf, err := newProdService(c.Context, hostname)
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
