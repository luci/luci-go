// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package logs

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/luci/luci-go/milo/common"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/router"
)

// Where it all begins!!!
func main() {
	r := router.New()

	base := common.FlexBase()
	r.GET("/log/raw/*path", base, rawLog)

	// Health check, for the appengine flex environment.
	http.HandleFunc("/_ah/health", healthCheckHandler)
	// And everything else.
	http.Handle("/", r)

	log.Print("Listening on port 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func page(c *router.Context, status int, msg string) {
	c.Writer.WriteHeader(status)
	fmt.Fprintf(c.Writer, msg)
}

func errorPage(c *router.Context, msg string) {
	page(c, http.StatusInternalServerError, msg)
}

func rawLog(c *router.Context) {
	path := c.Params.ByName("path")
	if path == "" {
		page(c, http.StatusBadRequest, "missing path")
		return
	}
	path = strings.TrimLeft(path, "/")
	host := c.Request.FormValue("host")
	if host == "" {
		host = "luci-logdog.appspot.com"
	}
	err := logHandler(c.Context, c.Writer, host, path)
	switch err {
	case nil:
		// Everything is fine
	case errNoAuth:
		// Redirect to login page
		loginURL, err := auth.LoginURL(c.Context, c.Request.URL.Path)
		if err != nil {
			fmt.Fprintf(c.Writer, "Encountered error generating login url: %s\n", err.Error())
			return
		}
		http.Redirect(c.Writer, c.Request, loginURL, http.StatusTemporaryRedirect)
		return
	default:
		fmt.Fprintf(c.Writer, "Encountered error: %s", err.Error())
	}
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "ok")
}
