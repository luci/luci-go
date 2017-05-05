// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package application

import (
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/errors"
)

var appKey = "github.com/luci/luci-go/vpython/application.A"

func withApplication(c context.Context, a *application) context.Context {
	return context.WithValue(c, &appKey, a)
}

func getApplication(c context.Context, args []string) *application {
	a := c.Value(&appKey).(*application)
	a.opts.Args = args
	return a
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
