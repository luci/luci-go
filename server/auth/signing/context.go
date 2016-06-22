// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package signing

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/luci/luci-go/server/router"
	"golang.org/x/net/context"
)

type contextKey int

// SetSigner injects Signer into the context.
func SetSigner(c context.Context, s Signer) context.Context {
	return context.WithValue(c, contextKey(0), s)
}

// GetSigner extracts Signer from the context. Returns nil if no Signer is set.
func GetSigner(c context.Context) Signer {
	if s, ok := c.Value(contextKey(0)).(Signer); ok {
		return s
	}
	return nil
}

// SignBytes signs the blob with some active private key using Signer installed
// in the context. Returns the signature and name of the key used.
func SignBytes(c context.Context, blob []byte) (keyName string, signature []byte, err error) {
	if s := GetSigner(c); s != nil {
		return s.SignBytes(c, blob)
	}
	return "", nil, errors.New("signature: no Signer in the context")
}

// InstallHandlers installs a handler that serves public certificates provided
// by the signer inside the base context. FetchCertificates is hitting this
// handler.
func InstallHandlers(r *router.Router, base router.MiddlewareChain) {
	r.GET("/auth/api/v1/server/certificates", base, certsHandler)
}

// certsHandler servers public certificates of the signer in the context.
func certsHandler(c *router.Context) {
	reply := func(code int, out interface{}) {
		c.Writer.Header().Set("Content-Type", "application/json")
		c.Writer.WriteHeader(code)
		json.NewEncoder(c.Writer).Encode(out)
	}

	replyError := func(code int, msg string) {
		errorReply := struct {
			Error string `json:"error"`
		}{msg}
		reply(code, &errorReply)
	}

	s := GetSigner(c.Context)
	if s == nil {
		replyError(http.StatusNotFound, "No Signer instance available")
		return
	}

	certs, err := s.Certificates(c.Context)
	if err != nil {
		replyError(http.StatusInternalServerError, fmt.Sprintf("Can't fetch certificates - %s", err))
	} else {
		reply(http.StatusOK, certs)
	}
}
