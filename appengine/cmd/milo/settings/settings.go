// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package settings

import (
	"fmt"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/milo/models"
	"github.com/luci/luci-go/appengine/cmd/milo/resp"
	"github.com/luci/luci-go/server/auth"
	"golang.org/x/net/context"
)

type updateReq struct {
	Theme string
}

// GetTheme returns the chosen theme based on the current user.
// TODO(hinoka): Cookie support
func GetTheme(c context.Context) Theme {
	cu := auth.CurrentUser(c)
	if cu == nil {
		return Default
	}
	userSettings := &models.UserConfig{UserID: cu.Identity}
	err := datastore.Get(c).Get(userSettings)
	if err != nil {
		return Default // Always return something, even if datastore is broken.
	}

	if t, ok := Themes[userSettings.Theme]; ok {
		return t
	}
	return Default
}

// TODO(hinoka): Anonymous users should use cookies as settings storage instead.
func settingsImpl(c context.Context, URL string, update *updateReq) (*resp.Settings, error) {
	// First get settings
	cu := auth.CurrentUser(c)
	ds := datastore.Get(c)
	userSettings := &models.UserConfig{UserID: cu.Identity}
	err := ds.Get(userSettings)
	if err != nil {
		if err != datastore.ErrNoSuchEntity {
			return nil, err
		}
	}

	// Do updatey stuff.
	if update != nil {
		// Deal with theme changes.
		if update.Theme != "" {
			if _, ok := Themes[update.Theme]; ok {
				userSettings.Theme = update.Theme
			} else {
				return nil, fmt.Errorf("Invalid theme %s", update.Theme)
			}
		}

		// Write it in!
		err := ds.Put(userSettings)
		if err != nil {
			return nil, err
		}
	}

	result := &resp.Settings{}

	result.ActionURL = URL

	result.Theme = &resp.Choices{
		Choices:  GetAllThemes(),
		Selected: userSettings.Theme,
	}

	return result, nil
}
