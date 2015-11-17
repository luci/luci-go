// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package model

import (
	"github.com/luci/luci-go/appengine/cmd/dm/display"
)

// Distributor is a simple model to whitelist distributors by name. Every
// distributor that quests may use must be registered by an admin first.
type Distributor struct {
	Name string `gae:"$id"`
	URL  string
}

// ToDisplay returns a display.Distributor for this Distributor.
func (d *Distributor) ToDisplay() *display.Distributor {
	return &display.Distributor{Name: d.Name, URL: d.URL}
}
