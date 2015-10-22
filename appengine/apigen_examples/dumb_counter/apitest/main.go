// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"errors"
	"flag"
	"net/http"
	"os"

	"github.com/luci/luci-go/appengine/apigen_examples/dumb_counter/api/dumb_counter/v1"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/gologger"
	"github.com/luci/luci-go/common/testing/httpmitm"
	"golang.org/x/net/context"
)

var (
	errOperationFailed = errors.New("operation failed")

	defaultBasePath string
)

type application struct {
	basePath string
}

func init() {
	// Determine the default base path by instantiating a default client.
	svc, _ := dumb_counter.New(http.DefaultClient)
	defaultBasePath = svc.BasePath
}

func (a *application) addToFlagSet(fs *flag.FlagSet) {
	fs.StringVar(&a.basePath, "base-path", defaultBasePath,
		"The URL to the base service. Leave black for default.")
}

func (a *application) run(c context.Context) error {
	client := http.Client{}
	client.Transport = &httpmitm.Transport{
		RoundTripper: http.DefaultTransport,
		Callback: func(o httpmitm.Origin, data []byte, err error) {
			log.Fields{
				log.ErrorKey: err,
				"origin":     o,
			}.Infof(c, "HTTP:\n%s", string(data))
		},
	}
	svc, err := dumb_counter.New(&client)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "Failed to create client.")
		return errOperationFailed
	}
	svc.BasePath = a.basePath

	resp, err := svc.Add("test", &dumb_counter.AddReq{Delta: 1}).Context(c).Do()
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "Failed to perform add operation.")
		return errOperationFailed
	}

	log.Fields{
		"current": resp.Cur,
		"prev":    resp.Prev,
	}.Infof(c, "Add operation successful!")
	return nil
}

func main() {
	a := application{}
	lc := log.Config{
		Level: log.Warning,
	}

	fs := flag.CommandLine
	a.addToFlagSet(fs)
	lc.AddFlags(fs)
	fs.Parse(os.Args[1:])

	ctx := context.Background()
	ctx = lc.Set(gologger.Use(ctx))

	if err := a.run(ctx); err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(ctx, "Error during execution.")
		os.Exit(1)
	}
}
