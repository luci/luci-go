// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prpc

import (
	"fmt"
	"net/http"

	"github.com/golang/protobuf/proto"
	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/luci/luci-go/server/middleware"
)

type method struct {
	service *service
	desc    grpc.MethodDesc
}

func (m *method) Name() string {
	return m.desc.MethodName
}

// Handle decodes an input protobuf message from the HTTP request,
// delegates RPC handling to the inner implementation and
// encodes the output message back to the HTTP response.
//
// If the inner handler returns an error, HTTP status is determined using
// ErrorStatus.
// If the status is http.StatusInternalServerError, only "Internal server error"
// is printed.
// All errors with status >= 500 are logged.
func (m *method) Handle(c context.Context, w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	if err := m.handle(c, w, r); err != nil {
		writeError(c, w, err)
	}
}

// handle decodes an input protobuf message from the HTTP request,
// delegates RPC handling to the inner implementation and
// encodes the output message back to the HTTP response.
func (m *method) handle(c context.Context, w http.ResponseWriter, r *http.Request) *httpError {
	defer r.Body.Close()
	format, err := responseFormat(r.Header.Get(headerAccept))
	if err != nil {
		return err
	}

	c, rawErr := parseHeader(c, r.Header)
	if rawErr != nil {
		return withStatus(rawErr, http.StatusBadRequest)
	}

	res, rawErr := m.desc.Handler(m.service.impl, c, func(msg interface{}) error {
		if msg == nil {
			panicf("cannot decode to nil")
		}
		// Do not collapse it to one line. There is implicit err type conversion.
		if err := readMessage(r, msg.(proto.Message)); err != nil {
			return err
		}
		return nil
	})
	if rawErr != nil {
		if err, ok := rawErr.(*httpError); ok {
			return err
		}
		return withStatus(rawErr, ErrorStatus(rawErr))
	}
	if res == nil {
		return m.internalServerError("service returned nil message")
	}
	if err := writeMessage(w, res.(proto.Message), format); err != nil {
		return m.internalServerError("could not respond: %s", err)
	}
	return nil
}

// InstallHandlers installs a POST HTTP handlers at /prpc/{service_name}/{method_name}.
func (m *method) InstallHandlers(r *httprouter.Router, base middleware.Base) {
	path := fmt.Sprintf("/prpc/%s/%s", m.service.Name(), m.Name())
	r.POST(path, base(m.Handle))
}

func (m *method) internalServerError(format string, a ...interface{}) *httpError {
	format = fmt.Sprintf("%s.%s: ", m.service.Name(), m.Name()) + format
	return errorf(http.StatusInternalServerError, format, a...)
}
