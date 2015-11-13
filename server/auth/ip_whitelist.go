// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package auth

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/auth/identity"
)

var (
	// ErrIPNotWhitelisted is returned when account is restricted by IP whitelist
	// and request' remote_addr is not in it.
	ErrIPNotWhitelisted = errors.New("auth: IP is not whitelisted")
)

// checkIPWhitelist returns new bot Identity if request was authenticated as
// coming from IP whitelisted bot.
//
// Returns ErrIPNotWhitelisted if identity has an IP whitelist assigned and
// given IP address doesn't belong to it.
func checkIPWhitelist(c context.Context, db DB, id identity.Identity, ip net.IP, h http.Header) (identity.Identity, error) {
	// Anonymous requests coming from IPs in 'bots' whitelist are authenticated
	// as bots.
	botIDHeader := h.Get("X-Whitelisted-Bot-Id")
	if id.Kind() == identity.Anonymous {
		whitelisted, err := db.IsInWhitelist(c, ip, "bots")
		if err != nil {
			return "", err
		}
		if whitelisted {
			botID := botIDHeader
			if botID == "" {
				botID = strings.Replace(ip.String(), ":", "-", -1)
			}
			return identity.MakeIdentity("bot:" + botID)
		}
	}

	// Only whitelisted bots are allowed to use this header.
	if botIDHeader != "" {
		return "", fmt.Errorf("X-Whitelisted-Bot-Id (%s) is used by bot not in the whitelist", botIDHeader)
	}

	// Not restricted by a whitelist?
	whitelist, err := db.GetWhitelistForIdentity(c, id)
	if err != nil {
		return "", err
	}
	if whitelist == "" {
		return id, err
	}

	// Return ErrIPNotWhitelisted is `ip` is not in the assigned whitelist.
	whitelisted, err := db.IsInWhitelist(c, ip, whitelist)
	if err == nil && !whitelisted {
		err = ErrIPNotWhitelisted
	}
	if err != nil {
		return "", err
	}

	return id, nil
}
