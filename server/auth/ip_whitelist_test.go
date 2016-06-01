// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package auth

import (
	"net"
	"net/http"
	"testing"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/auth/identity"
	"github.com/luci/luci-go/server/auth/service/protocol"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCheckIPWhitelist(t *testing.T) {
	Convey("checkIPWhitelist works", t, func() {
		c := context.Background()
		db, err := NewSnapshotDB(&protocol.AuthDB{
			IpWhitelistAssignments: []*protocol.AuthIPWhitelistAssignment{
				{
					Identity:    strPtr("user:abc@example.com"),
					IpWhitelist: strPtr("whitelist"),
				},
			},
			IpWhitelists: []*protocol.AuthIPWhitelist{
				{
					Name: strPtr("bots"),
					Subnets: []string{
						"10.10.10.10/32",
					},
				},
				{
					Name: strPtr("whitelist"),
					Subnets: []string{
						"1.2.3.4/32",
					},
				},
			},
		}, "http://auth-service", 1234)
		So(err, ShouldBeNil)

		callWithErr := func(id, ip, header string) (string, error) {
			ipaddr := net.ParseIP(ip)
			So(ipaddr, ShouldNotBeNil)
			headers := http.Header{}
			if header != "" {
				headers.Add("X-Whitelisted-Bot-Id", header)
			}
			out, err := checkIPWhitelist(c, db, identity.Identity(id), ipaddr, headers)
			return string(out), err
		}

		call := func(id, ip, header string) string {
			out, err := callWithErr(id, ip, header)
			So(err, ShouldBeNil)
			return out
		}

		// Not a bot.
		So(call("anonymous:anonymous", "123.123.123.123", ""), ShouldEqual, "anonymous:anonymous")

		// Whitelisted bot using authentication token.
		So(call("user:xxx@example.com", "10.10.10.10", ""), ShouldEqual, "user:xxx@example.com")

		// Whitelisted bot not using the header.
		So(call("anonymous:anonymous", "10.10.10.10", ""), ShouldEqual, "bot:10.10.10.10")

		// Whitelisted bot using the header.
		So(call("anonymous:anonymous", "10.10.10.10", "bot-id"), ShouldEqual, "bot:bot-id")

		// Non whitelisted bot using the header.
		_, err = callWithErr("anonymous:anonymous", "123.123.123.123", "bot-id")
		So(err, ShouldNotBeNil)

		// Account is restricted to IP and uses good IP.
		So(call("user:abc@example.com", "1.2.3.4", ""), ShouldEqual, "user:abc@example.com")

		// Account is restricted to IP and uses bad IP.
		_, err = callWithErr("user:abc@example.com", "1.1.1.1", "")
		So(err, ShouldEqual, ErrIPNotWhitelisted)
	})
}
