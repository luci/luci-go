// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/logging"
)

type format string

const (
	formatBinary format = "binary"
	formatJSON   format = "json"
	formatText   format = "text"
)

// TODO(nodir): support specifying message fields with options, e.g.
//   rpc call :8080 helloworld.Greeter -name Lucy
// It will be the default request format.

var cmdCall = &subcommands.Command{
	UsageLine: `call [flags] <server> <service>.<method>

  server: host ("example.com") or port for localhost (":8080").
  service: full name of a service, e.g. "pkg.service"
  method: name of the method.

Flags:`,
	ShortDesc: "calls a service method.",
	LongDesc:  "Calls a service method.",
	CommandRun: func() subcommands.CommandRun {
		c := &callRun{}
		c.registerBaseFlags()
		c.Flags.StringVar(&c.format, "format", string(formatJSON), fmt.Sprintf(
			"Request format, read from stdin. Valid values: %q, %q, %q.",
			formatJSON, formatBinary, formatText,
		))
		return c
	},
}

// callRun implements "call" subcommand.
type callRun struct {
	cmdRun
	format  string
	message string
}

func (r *callRun) Run(a subcommands.Application, args []string) int {
	if len(args) != 2 {
		return r.argErr("")
	}
	var req request
	var err error
	req.server, err = parseServer(args[0])
	if err != nil {
		return r.argErr("server: %s", err)
	}
	req.service, req.method, err = splitServiceAndMethod(args[1])
	if err != nil {
		return r.argErr("%s", err)
	}

	switch f := format(r.format); f {
	case formatJSON, formatBinary, formatText:
		req.format = f
	default:
		return r.argErr("invalid format %q", f)
	}

	var msg bytes.Buffer
	if _, err := io.Copy(&msg, os.Stdin); err != nil {
		return r.done(fmt.Errorf("could not read message from stdin: %s", err))
	}
	req.message = msg.Bytes()
	return r.done(call(r.initContext(), &req, os.Stdout))
}

func splitServiceAndMethod(fullName string) (service string, method string, err error) {
	lastDot := strings.LastIndex(fullName, ".")
	if lastDot < 0 {
		return "", "", fmt.Errorf("invalid full method name %q. It must contain a '.'", fullName)
	}
	service = fullName[:lastDot]
	method = fullName[lastDot+1:]
	return
}

// request is an RPC request.
type request struct {
	server  *url.URL
	service string
	method  string
	format  format
	message []byte
}

// call makes an RPC and writes response to out.
func call(c context.Context, req *request, out io.Writer) error {
	if req.format == "" {
		return fmt.Errorf("format is not set")
	}

	// Create HTTP request.
	methodURL := *req.server
	methodURL.Path = fmt.Sprintf("/prpc/%s/%s", req.service, req.method)
	hr, err := http.NewRequest("POST", methodURL.String(), bytes.NewBuffer(req.message))
	if err != nil {
		return err
	}

	// Set headers.
	mediaType := "application/prpc; encoding=" + string(req.format)
	if len(req.message) > 0 {
		hr.Header.Set("Content-Type", mediaType)
	}
	hr.Header.Set("Accept", mediaType)
	hr.Header.Set("User-Agent", userAgent)
	hr.Header.Set("Content-Length", fmt.Sprintf("%d", len(req.message)))
	// TODO(nodir): add "Accept-Encoding: gzip" when pRPC server supports it.

	// Log request in curl style.
	logRequest(c, hr)
	logging.Infof(c, ">")

	// Send the request.
	var client http.Client
	res, err := client.Do(hr)
	if err != nil {
		return fmt.Errorf("failed to send request: %s", err)
	}
	defer res.Body.Close()

	// Log response in curl style.
	logResponse(c, res)
	logging.Infof(c, "<")

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%s", res.Status)
	}

	// Read response.
	if _, err := io.Copy(out, res.Body); err != nil {
		return fmt.Errorf("failed to read response: %s", err)
	}
	return err
}

// logRequest logs an HTTP request in curl style.
func logRequest(c context.Context, r *http.Request) {
	logging.Infof(c, "> %s %s %s", r.Method, r.URL.Path, r.Proto)
	logging.Infof(c, "> Host: %s", r.URL.Host)
	logHeaders(c, r.Header, "> ")
}

// logResponse logs an HTTP response in curl style.
func logResponse(c context.Context, r *http.Response) {
	logging.Infof(c, "< %s %s", r.Proto, r.Status)
	logHeaders(c, r.Header, "< ")

}

// logHeaders logs all headers sorted.
func logHeaders(c context.Context, header http.Header, prefix string) {
	var names []string
	for n := range header {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, v := range header[name] {
			logging.Infof(c, "%s%s: %s", prefix, name, v)
		}
	}
}
