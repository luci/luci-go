// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package epfrontend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
)

type outerError struct {
	Error *innerError `json:"error"`
}

type innerError struct {
	Code    int              `json:"code"`
	Errors  []*errorInstance `json:"errors"`
	Message string           `json:"message"`
}

type errorInstance struct {
	Domain  string `json:"domain"`
	Message string `json:"message"`
	Reason  string `json:"reason"`
}

// endpointsErrorResponse is a copy of go-endpoints' errorResponse struct.
type endpointsErrorResponse struct {
	// Currently always "APPLICATION_ERROR"
	State string `json:"state"`
	Name  string `json:"error_name"`
	Msg   string `json:"error_message,omitempty"`
	Code  int    `json:"-"`
}

// errorResponseWriter is an http.ResponseWriter implementation that captures
// endpoints errors that are written to it and emits them as error JSON.
//
// This fulfills the purpose of collecting an error response, generated either
// by the frontend processing code (e.g., could not find method) or the backend
// handling code (e.g., endpoint returned an error, JSON unmarshal, etc.) and
// boxing it into a frontend error response structure.
//
// If the frontend handler encounters an error, it will be set via setError.
//
// If the backend code generates an error, it will be captured as follows:
// 1) The backend will invoke WriteHeader with an error status. That will enable
//    response buffering.
// 2) Write will be called to write out the backend error response JSON. This
//    will be captured for deconstruction.
//
// In either case, the error state will be forwarded to the real underlying
// ResponseWriter via forwardError. If no error is assigned, all operations
// essentially pass through to the underlying ResponseWriter, making this very
// low overhead in the standard case.
type errorResponseWriter struct {
	http.ResponseWriter

	status int
	inst   *errorInstance
	buf    bytes.Buffer
}

func (w *errorResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}

	// If we have an error status (>= 400)
	w.status = status
	if w.status >= http.StatusBadRequest {
		w.status, w.inst = w.translateReason(status)
	}
	w.ResponseWriter.WriteHeader(w.status)
}

func (w *errorResponseWriter) Write(d []byte) (int, error) {
	// If we have an error status.
	if w.inst != nil {
		return w.buf.Write(d)
	}
	return w.ResponseWriter.Write(d)
}

// setError explicitly configures an error response.
//
// This is used by the frontend ServeHTTP code when an error is encountered
// prior to calling into the backend. If the backend is invoked, setError will
// not be called.
func (w *errorResponseWriter) setError(err error) {
	e, ok := err.(*endpoints.APIError)
	if !ok {
		e = &endpoints.APIError{
			Msg:  err.Error(),
			Code: http.StatusInternalServerError,
		}
	}
	w.WriteHeader(e.Code)
	w.inst.Message = err.Error()
}

func (w *errorResponseWriter) forwardError() bool {
	if w.inst == nil {
		// No buffered error; leave things be.
		return false
	}

	ierr := innerError{
		Code:    w.status,
		Message: "unspecified error",
		Errors:  []*errorInstance{w.inst},
	}
	if w.inst.Message == "" {
		// Attempt to load the message from the backend endpoints error JSON.
		if w.buf.Len() > 0 {
			resp := endpointsErrorResponse{}
			if err := json.NewDecoder(&w.buf).Decode(&resp); err != nil {
				w.inst.Message = fmt.Sprintf("Failed to decode error JSON (%s): %s", err, w.buf.String())
			} else {
				w.inst.Message = resp.Msg
				ierr.Message = resp.Msg
			}
		}
	}

	feErr := outerError{
		Error: &ierr,
	}
	data, err := json.MarshalIndent(feErr, "", " ")
	if err != nil {
		return true
	}
	w.ResponseWriter.Write(data)
	return true
}

// translateReason returns an errorInstance populated from an HTTP status.
//
// The mappings here are copied from Python AppEngine:
// /google/appengine/tools/devappserver2/endpoints/generated_error_info.py
func (*errorResponseWriter) translateReason(status int) (int, *errorInstance) {
	code := status
	r := errorInstance{
		Domain: "global",
	}
	switch status {
	case 400:
		r.Reason = "badRequest"
	case 401:
		r.Reason = "required"
	case 402:
		r.Reason = "unsupportedProtocol"
	case 403:
		r.Reason = "forbidden"
	case 404:
		r.Reason = "notFound"
	case 405:
		r.Reason = "unsupportedMethod"
	case 409:
		r.Reason = "conflict"
	case 410:
		r.Reason = "deleted"
	case 412:
		r.Reason = "conditionNotMet"
	case 413:
		r.Reason = "uploadTooLarge"

	case 406, 407, 411, 414, 415, 416, 417:
		code = 404
		r.Reason = "unsupportedProtocol"

	case 408:
		fallthrough
	default:
		code = 503
		r.Reason = "backendError"
	}
	return code, &r
}
