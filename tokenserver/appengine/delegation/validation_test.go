// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package delegation

import (
	"testing"

	"github.com/golang/protobuf/proto"

	admin "github.com/luci/luci-go/tokenserver/api/admin/v1"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestValidation(t *testing.T) {
	cases := []struct {
		Cfg    string
		Errors []string
	}{
		{
			// No errors, "normal looking" config.
			Cfg: `
				rules {
					name: "rule 1"
					requestor: "user:some-app@appspot.gserviceaccount.com"
					target_service: "service:some-service"
					allowed_to_impersonate: "group:some-group"
					allowed_audience: "REQUESTOR"
					max_validity_duration: 86400
				}

				rules {
					name: "rule 2"
					requestor: "group:some-group"
					target_service: "*"
					allowed_to_impersonate: "group:another-group"
					allowed_audience: "*"
					max_validity_duration: 86400
				}
			`,
		},

		{
			// Duplicate names.
			Cfg: `
				rules {
					name: "rule 1"
					requestor: "user:some-app@appspot.gserviceaccount.com"
					target_service: "service:some-service"
					allowed_to_impersonate: "group:some-group"
					allowed_audience: "REQUESTOR"
					max_validity_duration: 86400
				}

				rules {
					name: "rule 1"
					requestor: "group:some-group"
					target_service: "*"
					allowed_to_impersonate: "group:another-group"
					allowed_audience: "*"
					max_validity_duration: 86400
				}
			`,
			Errors: []string{`rule #2 ("rule 1"): the rule with such name is already defined`},
		},

		{
			// Missing required fields.
			Cfg: `
				rules {
				}
			`,
			Errors: []string{
				`'name' is required`,
				`'requestor' is required`,
				`'allowed_to_impersonate' is required`,
				`'allowed_audience' is required`,
				`'target_service' is required`,
				`'max_validity_duration' is required`,
			},
		},

		{
			// Validity duration out of range.
			Cfg: `
				rules {
					name: "rule 1"
					requestor: "user:some-app@appspot.gserviceaccount.com"
					target_service: "service:some-service"
					allowed_to_impersonate: "group:some-group"
					allowed_audience: "REQUESTOR"
					max_validity_duration: -1
				}
				rules {
					name: "rule 2"
					requestor: "user:some-app@appspot.gserviceaccount.com"
					target_service: "service:some-service"
					allowed_to_impersonate: "group:some-group"
					allowed_audience: "REQUESTOR"
					max_validity_duration: 86401
				}
			`,
			Errors: []string{
				`rule #1 ("rule 1"): 'max_validity_duration' must be positive`,
				`rule #2 ("rule 2"): 'max_validity_duration' must be smaller than 86401`,
			},
		},

		{
			// Bad requestor.
			Cfg: `
				rules {
					name: "rule 1"
					requestor: "user:some-app@appspot.gserviceaccount.com" # ok
					requestor: "service:blah" # ok
					requestor: "group:some-group" # ok
					requestor: "*" # not ok
					requestor: "some junk" # not ok
					requestor: "group:" # not ok
					target_service: "service:some-service"
					allowed_to_impersonate: "group:some-group"
					allowed_audience: "REQUESTOR"
					max_validity_duration: 3600
				}
			`,
			Errors: []string{
				`bad 'requestor' - auth: bad identity string "*"`,
				`bad 'requestor' - auth: bad identity string "some junk"`,
				`bad 'requestor' - bad group entry "group:"`,
			},
		},

		{
			// Bad allowed_to_impersonate.
			Cfg: `
				rules {
					name: "rule 1"
					requestor: "user:some-app@appspot.gserviceaccount.com"
					target_service: "service:some-service"
					allowed_to_impersonate: "user:abc@example.com" # ok
					allowed_to_impersonate: "group:some-group" # ok
					allowed_to_impersonate: "REQUESTOR" # ok
					allowed_to_impersonate: "*" # not OK
					allowed_audience: "REQUESTOR"
					max_validity_duration: 86400
				}
			`,
			Errors: []string{
				`bad 'allowed_to_impersonate' - auth: bad identity string "*"`,
			},
		},

		{
			// Bad allowed_audience.
			Cfg: `
				rules {
					name: "rule 1"
					requestor: "user:some-app@appspot.gserviceaccount.com"
					target_service: "service:some-service"
					allowed_to_impersonate: "user:abc@example.com"
					allowed_audience: "REQUESTOR" # ok
					allowed_audience: "*" # ok
					allowed_audience: "user:abc@example.com" # ok
					allowed_audience: "group:abc" # ok
					allowed_audience: "some junk" # not ok
					max_validity_duration: 86400
				}
			`,
			Errors: []string{
				`bad 'allowed_audience' - auth: bad identity string "some junk"`,
			},
		},

		{
			// Bad target_service.
			Cfg: `
				rules {
					name: "rule 1"
					requestor: "user:some-app@appspot.gserviceaccount.com"
					target_service: "service:some-service" # ok
					target_service: "user:abc@example.com" # not ok
					target_service: "group:some-group" # not ok
					allowed_to_impersonate: "user:abc@example.com"
					allowed_audience: "REQUESTOR"
					max_validity_duration: 86400
				}
			`,
			Errors: []string{
				`bad 'target_service' - identity of kind "user" is not allowed here`,
				`bad 'target_service' - group entries are not allowe`,
			},
		},
	}

	Convey("Validation works", t, func(c C) {
		for idx, cs := range cases {
			c.Printf("Case #%d\n", idx)
			cfg := &admin.DelegationPermissions{}
			err := proto.UnmarshalText(cs.Cfg, cfg)
			So(err, ShouldBeNil)
			merr := ValidateConfig(cfg)
			So(len(merr), ShouldEqual, len(cs.Errors))
			for i, err := range merr {
				So(err, ShouldErrLike, cs.Errors[i])
			}
		}
	})
}
