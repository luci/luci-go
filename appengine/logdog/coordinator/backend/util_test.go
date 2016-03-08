// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package backend

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/julienschmidt/httprouter"
	tq "github.com/luci/gae/service/taskqueue"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/server/middleware"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

// testBase is a middleware.Base which uses its current Context as the base
// context.
type testBase struct {
	context.Context
}

func (t *testBase) base(h middleware.Handler) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		h(t.Context, w, r, p)
	}
}

func shouldHaveTasks(actual interface{}, expected ...interface{}) string {
	a := actual.(map[string]*tq.Task)
	al := make([]string, 0, len(a))
	for _, t := range a {
		al = append(al, t.Name)
	}

	tasks := make([]string, len(expected))
	for i, t := range expected {
		tasks[i] = t.(string)
	}

	sort.Strings(al)
	sort.Strings(tasks)
	return ShouldResemble(al, tasks)
}

func archiveTaskName(path string) string {
	return fmt.Sprintf("archive-%s", coordinator.LogStreamFromPath(types.StreamPath(path)).HashID())
}

func cleanupTaskName(path string) string {
	return fmt.Sprintf("cleanup-%s", coordinator.LogStreamFromPath(types.StreamPath(path)).HashID())
}
