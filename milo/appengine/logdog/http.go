// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package logdog

import (
	"net/http"
	"strings"

	"github.com/luci/luci-go/common/errors"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/grpc/prpc"
	"github.com/luci/luci-go/logdog/client/coordinator"
	"github.com/luci/luci-go/logdog/common/types"
	"github.com/luci/luci-go/luci_config/common/cfgtypes"
	"github.com/luci/luci-go/milo/appengine/settings"
	"github.com/luci/luci-go/milo/common/miloerror"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/templates"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"
)

// AnnotationStreamHandler is a ThemedHandler that renders a LogDog Milo
// annotation protobuf stream.
//
// The protobuf stream is fetched live from LogDog and cached locally, either
// temporarily (if incomplete) or indefinitely (if complete).
type AnnotationStreamHandler struct{}

// GetTemplateName implements settings.ThemedHandler.
func (s *AnnotationStreamHandler) GetTemplateName(t settings.Theme) string {
	return "build.html"
}

// Render implements settings.ThemedHandler.
func (s *AnnotationStreamHandler) Render(c context.Context, req *http.Request, p httprouter.Params) (
	*templates.Args, error) {

	as := AnnotationStream{
		Project: cfgtypes.ProjectName(p.ByName("project")),
		Path:    types.StreamPath(strings.Trim(p.ByName("path"), "/")),
	}
	if err := as.Normalize(); err != nil {
		return nil, &miloerror.Error{
			Message: err.Error(),
			Code:    http.StatusBadRequest,
		}
	}

	// Setup our LogDog client.
	var err error
	if as.Client, err = NewClient(c, ""); err != nil {
		log.WithError(err).Errorf(c, "Failed to generate LogDog client.")
		return nil, &miloerror.Error{
			Code: http.StatusInternalServerError,
		}
	}

	// Load the Milo annotation protobuf from the annotation stream.
	if _, err := as.Fetch(c); err != nil {
		switch errors.Unwrap(err) {
		case coordinator.ErrNoSuchStream:
			return nil, &miloerror.Error{
				Message: "Stream does not exist",
				Code:    http.StatusNotFound,
			}

		case coordinator.ErrNoAccess:
			return nil, &miloerror.Error{
				Message: "No access to stream",
				Code:    http.StatusForbidden,
			}

		default:
			return nil, &miloerror.Error{
				Message: "Failed to load stream",
				Code:    http.StatusInternalServerError,
			}
		}
	}

	// Convert the Milo Annotation protobuf to Milo objects.
	return &templates.Args{
		"Build": as.toMiloBuild(c),
	}, nil
}

func resolveHost(host string) (string, error) {
	// Resolveour our Host, and validate it against a host whitelist.
	switch host {
	case "":
		return defaultLogDogHost, nil
	case defaultLogDogHost, "luci-logdog-dev.appspot.com":
		return host, nil
	default:
		return "", errors.Reason("host %(host)q is not whitelisted").
			D("host", host).
			Err()
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
