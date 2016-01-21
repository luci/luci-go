// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"net/http"
	"time"

	"github.com/luci/luci-go/common/auth"
	"golang.org/x/net/context"
)

const clientKey = "client"

type clientImpl struct {
	http.Client
	auth *auth.Authenticator
}

// hasToken returns true if access token is known.
// May return false negatives.
func (c *clientImpl) hasToken() bool {
	if c.auth == nil {
		return false
	}
	_, err := c.auth.GetAccessToken(30 * time.Second)
	return err == nil
}

func getClient(c context.Context) *clientImpl {
	client := c.Value(clientKey).(*clientImpl)
	if client == nil {
		client = &clientImpl{}
	}
	return client
}
