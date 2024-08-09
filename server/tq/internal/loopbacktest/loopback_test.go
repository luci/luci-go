// Copyright 2022 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package loopbacktest

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/auth/integration/authtest"
	"go.chromium.org/luci/auth/integration/localauth"
	"go.chromium.org/luci/lucictx"

	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/tq"
)

func TestLoopbackHTTPExecutor(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	incomingTasks := make(chan proto.Message)

	// Register a task class.
	disp := tq.Dispatcher{}
	disp.RegisterTaskClass(tq.TaskClass{
		ID:        "test-dur",
		Prototype: &durationpb.Duration{}, // just some proto type
		Kind:      tq.NonTransactional,
		Queue:     "queue-1",
		Handler: func(ctx context.Context, payload proto.Message) error {
			incomingTasks <- payload
			return nil
		},
	})

	// The server needs an auth context to run in (it won't actually use any
	// tokens though in this particular test).
	authSrv := localauth.Server{
		TokenGenerators: map[string]localauth.TokenGenerator{
			"authtest": &authtest.FakeTokenGenerator{Email: "test@example.com"},
		},
		DefaultAccountID: "authtest",
	}
	la, err := authSrv.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to launch localauth.Server: %s", err)
	}
	ctx = lucictx.SetLocalAuth(ctx, la)
	t.Cleanup(func() { _ = authSrv.Stop(ctx) })

	// Actually run the server with the TQ module.
	srv, err := server.New(ctx, server.Options{
		Prod:      false,
		HTTPAddr:  "127.0.0.1:0",
		AdminAddr: "-",
	}, []module.Module{
		tq.NewModule(&tq.ModuleOptions{
			Dispatcher:    &disp,
			ServingPrefix: "/internal/tasks",
			SweepMode:     "inproc",
		}),
	})
	if err != nil {
		t.Fatalf("failed to initialize the server: %s", err)
	}

	// Run its loop in background, then kill it.
	go func() { _ = srv.Serve() }()
	defer srv.Shutdown()

	const TaskCount = 5

	// Emit a bunch of tasks via the submitter assigned to the server (it lives
	// in the server's context).
	for i := time.Duration(0); i < time.Duration(TaskCount); i++ {
		err = disp.AddTask(srv.Context, &tq.Task{Payload: durationpb.New(i)})
		if err != nil {
			t.Fatalf("failed to add a task: %s", err)
		}
	}

	// Make sure they eventually are handled.
	seen := map[time.Duration]struct{}{}
	for i := 0; i < TaskCount; i++ {
		select {
		case got := <-incomingTasks:
			seen[got.(*durationpb.Duration).AsDuration()] = struct{}{}
		case <-time.After(time.Minute): // gross overestimate to deflake
			t.Fatalf("timeout while waiting for a task handler to be called")
		}
	}
	if len(seen) != TaskCount {
		t.Fatalf("expected %d tasks, got %v", TaskCount, seen)
	}
}
