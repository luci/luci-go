// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package raw_presentation

import (
	"net/http"
	"strings"

	"github.com/luci/luci-go/common/errors"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/grpc/prpc"
	"github.com/luci/luci-go/logdog/client/coordinator"
	"github.com/luci/luci-go/logdog/common/types"
	"github.com/luci/luci-go/luci_config/common/cfgtypes"
	"github.com/luci/luci-go/milo/common"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/router"
	"github.com/luci/luci-go/server/templates"

	"golang.org/x/net/context"
)

// AnnotationStreamHandler is a Handler that renders a LogDog Milo
// annotation protobuf stream.
//
// The protobuf stream is fetched live from LogDog and cached locally, either
// temporarily (if incomplete) or indefinitely (if complete).
type AnnotationStreamHandler struct{}

func BuildHandler(c *router.Context) {
	(&AnnotationStreamHandler{}).Render(c)
	return
}

// Render implements settings.ThemedHandler.
func (s *AnnotationStreamHandler) Render(c *router.Context) {
	as := AnnotationStream{
		Project: cfgtypes.ProjectName(c.Params.ByName("project")),
		Path:    types.StreamPath(strings.Trim(c.Params.ByName("path"), "/")),
	}
	if err := as.Normalize(); err != nil {
		common.ErrorPage(c, http.StatusBadRequest, err.Error())
		return
	}

	// Setup our LogDog client.
	var err error
	host := strings.TrimSpace(c.Params.ByName("logdog_host"))
	if as.Client, err = NewClient(c.Context, host); err != nil {
		log.WithError(err).Errorf(c.Context, "Failed to generate LogDog client.")
		common.ErrorPage(c, http.StatusInternalServerError, "Failed to generate LogDog client")
		return
	}

	// Load the Milo annotation protobuf from the annotation stream.
	if _, err := as.Fetch(c.Context); err != nil {
		switch errors.Unwrap(err) {
		case coordinator.ErrNoSuchStream:
			common.ErrorPage(c, http.StatusNotFound, "Stream does not exist")

		case coordinator.ErrNoAccess:
			common.ErrorPage(c, http.StatusForbidden, "No access to LogDog stream")

		case errNotMilo, errNotDatagram:
			// The user requested a LogDog url that isn't a Milo annotation.
			common.ErrorPage(c, http.StatusBadRequest, err.Error())

		default:
			log.WithError(err).Errorf(c.Context, "Failed to load LogDog stream.")
			common.ErrorPage(c, http.StatusInternalServerError, "Failed to load LogDog stream")
		}
		return
	}

	templates.MustRender(c.Context, c.Writer, "pages/build.html", templates.Args{
		"Build": as.toMiloBuild(c.Context),
	})
}

func resolveHost(host string) (string, error) {
	// Resolveour our Host, and validate it against a host whitelist.
	switch host {
	case "":
		return defaultLogDogHost, nil
	case defaultLogDogHost, "luci-logdog-dev.appspot.com":
		return host, nil
	default:
		return "", errors.Reason("host %q is not whitelisted", host).Err()
	}
}

// NewClient generates a new LogDog client that issues requests on behalf of the
// current user.
func NewClient(c context.Context, host string) (*coordinator.Client, error) {
	var err error
	if host, err = resolveHost(host); err != nil {
		return nil, err
	}

	// Initialize the LogDog client authentication.
	t, err := auth.GetRPCTransport(c, auth.AsUser)
	if err != nil {
		return nil, errors.New("failed to get transport for LogDog server")
	}

	// Setup our LogDog client.
	return coordinator.NewClient(&prpc.Client{
		C: &http.Client{
			Transport: t,
		},
		Host: host,
	}), nil
}
