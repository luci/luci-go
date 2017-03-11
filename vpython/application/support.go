// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package application

import (
	"github.com/luci/luci-go/common/errors"

	"golang.org/x/net/context"
)

var appKey = "github.com/luci/luci-go/vpython/application.A"

func withConfig(c context.Context, cfg *Config) context.Context {
	return context.WithValue(c, &appKey, cfg)
}

func getConfig(c context.Context) *Config {
	return c.Value(&appKey).(*Config)
}

func run(c context.Context, fn func(context.Context) error) int {
	err := fn(c)

	switch t := errors.Unwrap(err).(type) {
	case nil:
		return 0

	case ReturnCodeError:
		return int(t)

	default:
		errors.Log(c, err)
		return 1
	}
}
