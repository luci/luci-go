// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// eventlog is an example that demonstrates how to log to the eventlog service.
// It logs to a locally-run server. See go/localeventlog for more information.
package main

import (
	"fmt"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/eventlog"
	logpb "github.com/luci/luci-go/common/eventlog/proto"
)

func main() {
	ctx := context.Background()
	c := eventlog.NewClient(ctx, "http://localhost:27910/log")

	event := c.NewLogEvent(ctx, eventlog.Point())

	// Log a build event. Other kinds of events may also be logged.
	event.InfraEvent.BuildEvent = &logpb.BuildEvent{ /* TODO:fill in contents */ }

	err := c.LogSync(ctx, event)
	if err != nil {
		fmt.Printf("logging: %v\n", err)
	}
}
