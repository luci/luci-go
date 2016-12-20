// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package oauth2 provides instrumentation for the "golang.org/x/oauth2" and
// "github.com/luci/luci-go/common/auth" packages (which uses oauth2 internally)
package oauth2

import (
	"net/http"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/tsmon/metric"

	baseOAuth2 "golang.org/x/oauth2"
)

// Instrument instruments the HTTP client transport used by the oauth2 package
// inside the given context.  The new transport repors tsmon metrics about the
// requests that are made using it.
func Instrument(ctx context.Context) context.Context {
	// Make a copy the existing client from the context, if any.
	c := *baseOAuth2.NewClient(ctx, nil)

	// Wrap the transport inside.
	if c.Transport == nil {
		c.Transport = http.DefaultTransport
	}
	c.Transport = metric.InstrumentTransport(ctx, c.Transport, "oauth2")

	// Put it back in the context.
	return context.WithValue(ctx, baseOAuth2.HTTPClient, &c)
}
