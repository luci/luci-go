// Copyright 2023 The LUCI Authors.
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

// Command fakebot calls Swarming RBE API endpoints to test them.
//
// It is intended to be running locally side-by-side with a Swarming RBE
// server process that is running in integration testing mode with
// `-expose-integration-mocks` flag passed to it.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/system/signals"
	"go.chromium.org/luci/grpc/prpc"

	internalspb "go.chromium.org/luci/swarming/proto/internals"
	"go.chromium.org/luci/swarming/server/rbe"
	"go.chromium.org/luci/swarming/server/testing/integrationmocks"
)

var (
	botID       = flag.String("bot-id", "fake-bot", "ID of this bot")
	pool        = flag.String("pool", "local-test", "Value for `pool` dimension")
	serverPort  = flag.Int("server-port", 8800, "Localhost port with the Swarming RBE server")
	rbeInstance = flag.String("rbe-instance", "projects/chromium-swarm-dev/instances/default_instance", "Full RBE instance name to use for tests")
	taskDelay   = flag.Duration("task-delay", 5*time.Second, "How long to pretend working on a task")
)

func main() {
	flag.Parse()
	ctx := gologger.StdConfig.Use(context.Background())
	if err := run(ctx); err != nil {
		errors.Log(ctx, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	loopCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	signals.HandleInterrupt(func() {
		logging.Infof(ctx, "Got termination signal")
		cancel()
	})

	bot := NewBot(ctx, *botID, *pool, *serverPort, *rbeInstance)

	// The last processed lease we should close on the next session update.
	var lease *rbe.Lease

	defer func() {
		// Always try to cleanup the session when exiting.
		if bot.HasSession() {
			if _, err := bot.UpdateSession(ctx, true, "BOT_TERMINATING", lease); err != nil {
				logging.Errorf(ctx, "Error when terminating the bot session: %s", err)
			}
		}
	}()

	for loopCtx.Err() == nil {
		// Pretend to poll Swarming Python to get the fresh poll token.
		if err := bot.RefreshPollToken(ctx); err != nil {
			return errors.Annotate(err, "getting poll token").Err()
		}

		// Create a new session if there's no current healthy session.
		if !bot.HasSession() {
			if err := bot.CreateSession(ctx); err != nil {
				return errors.Annotate(err, "creating bot session").Err()
			}
		}

		// Wait for a lease. This also closes the previous lease, if any.
		var err error
		switch lease, err = bot.UpdateSession(ctx, true, "OK", lease); {
		case err != nil:
			return errors.Annotate(err, "polling for a task").Err()
		case !bot.HasSession():
			logging.Errorf(ctx, "Bot session was closed by the server")
			lease = nil
		case lease != nil:
			// If got a lease, launch the worker loop.
			lease, err = workerLoop(ctx, loopCtx, bot, lease)
			if err != nil {
				return errors.Annotate(err, "when working on a lease").Err()
			}
		}
	}

	return nil
}

func workerLoop(ctx, loopCtx context.Context, bot *Bot, lease *rbe.Lease) (*rbe.Lease, error) {
	if lease.State != "PENDING" {
		return nil, errors.Reason("unexpected lease state %s", lease.State).Err()
	}

	leaseID := lease.ID
	payload := lease.Payload

	if payload.Noop {
		return &rbe.Lease{
			ID:     leaseID,
			State:  "COMPLETED",
			Result: &internalspb.TaskResult{},
		}, nil
	}

	loopCtx, cancel := clock.WithTimeout(loopCtx, *taskDelay)
	defer cancel()

	for loopCtx.Err() == nil {
		// "Ping" the lease. This also tells us if we should drop it.
		lease, err := bot.UpdateSession(ctx, false, "OK", &rbe.Lease{
			ID:    leaseID,
			State: "ACTIVE",
		})
		switch {
		case err != nil:
			return nil, errors.Annotate(err, "when pinging lease").Err()
		case lease == nil:
			return nil, errors.Reason("the lease disappeared").Err()
		case lease.ID != leaseID:
			return nil, errors.Reason("got unexpected lease %q != %q", lease.ID, leaseID).Err()
		case lease.State == "ACTIVE":
			// Carry on.
		case lease.State == "CANCELLED":
			// Done with this lease.
			logging.Infof(ctx, "The lease was canceled")
			return &rbe.Lease{
				ID:    leaseID,
				State: "COMPLETED",
			}, nil
		default:
			return nil, errors.Reason("got unexpected lease state %s", lease.State).Err()
		}
		clock.Sleep(loopCtx, time.Second)
	}

	return &rbe.Lease{
		ID:     leaseID,
		State:  "COMPLETED",
		Result: &internalspb.TaskResult{},
	}, nil
}

////////////////////////////////////////////////////////////////////////////////

type Bot struct {
	dimensions  map[string][]string
	server      string
	rbeInstance string

	mocks integrationmocks.IntegrationMocksClient

	pollToken     []byte
	nextPollToken time.Time

	sessionToken  []byte
	sessionID     string
	sessionStatus string
	sessionExpiry time.Time
}

func NewBot(ctx context.Context, botID, pool string, serverPort int, rbeInstance string) *Bot {
	return &Bot{
		dimensions: map[string][]string{
			"id":   {botID},
			"pool": {pool},
		},
		server:      fmt.Sprintf("http://127.0.0.1:%d", serverPort),
		rbeInstance: rbeInstance,
		mocks: integrationmocks.NewIntegrationMocksClient(&prpc.Client{
			C:       http.DefaultClient,
			Host:    fmt.Sprintf("127.0.0.1:%d", serverPort),
			Options: &prpc.Options{Insecure: true},
		}),
	}
}

// rpc sends a JSON RPC to Swarming RBE local server.
func (b *Bot) rpc(ctx context.Context, endpoint string, req, resp any) error {
	blob, err := json.Marshal(req)
	if err != nil {
		return errors.Annotate(err, "failed to marshal the request body").Err()
	}

	httpResp, err := http.DefaultClient.Post(
		b.server+endpoint,
		"application/json; charset=utf-8",
		bytes.NewReader(blob))

	var respBody []byte
	if httpResp != nil && httpResp.Body != nil {
		defer func() { _ = httpResp.Body.Close() }()
		var err error
		if respBody, err = io.ReadAll(httpResp.Body); err != nil {
			return errors.Annotate(err, "failed to read response body").Err()
		}
	}

	if err != nil {
		return errors.Annotate(err, "%s", endpoint).Err()
	}
	if httpResp.StatusCode != http.StatusOK {
		return errors.Reason("%s: HTTP %d: %s", endpoint, httpResp.StatusCode, string(respBody)).Err()
	}

	if resp != nil && respBody != nil {
		if err := json.Unmarshal(respBody, resp); err != nil {
			return errors.Annotate(err, "failed to unmarshal the response (%s): %q", err, string(respBody)).Err()
		}
	}

	return nil
}

// RefreshPollToken grabs a fresh poll token if necessary.
func (b *Bot) RefreshPollToken(ctx context.Context) error {
	if !b.nextPollToken.IsZero() && clock.Now(ctx).Before(b.nextPollToken) {
		return nil
	}

	tok, err := b.mocks.GeneratePollToken(ctx, &internalspb.PollState{
		Id: "fake-token",
		EnforcedDimensions: []*internalspb.PollState_Dimension{
			{Key: "id", Values: b.dimensions["id"]},
		},
		Expiry:      timestamppb.New(clock.Now(ctx).Add(time.Hour)),
		RbeInstance: b.rbeInstance,
		IpAllowlist: "localhost",
		AuthMethod: &internalspb.PollState_IpAllowlistAuth{
			IpAllowlistAuth: &internalspb.PollState_IPAllowlistAuth{},
		},
	})
	if err != nil {
		return err
	}

	b.pollToken = tok.PollToken
	b.nextPollToken = clock.Now(ctx).Add(time.Minute)

	return nil
}

// HasSession is true if we have an active session.
func (b *Bot) HasSession() bool {
	return len(b.sessionToken) != 0 && b.sessionStatus == "OK"
}

// CreateSession creates a new bot session.
func (b *Bot) CreateSession(ctx context.Context) error {
	logging.Infof(ctx, "Creating the session")

	var resp rbe.CreateBotSessionResponse
	err := b.rpc(ctx, "/swarming/api/v1/bot/rbe/session/create", &rbe.CreateBotSessionRequest{
		PollToken:  b.pollToken,
		Dimensions: b.dimensions,
	}, &resp)
	if err != nil {
		return errors.Annotate(err, "creating session").Err()
	}

	b.sessionToken = resp.SessionToken
	b.sessionID = resp.SessionID
	b.sessionStatus = "OK"
	b.sessionExpiry = time.Unix(resp.SessionExpiry, 0).UTC()

	logging.Infof(ctx, "Created the session: %s", b.sessionID)
	return nil
}

// UpdateSession updates the session.
func (b *Bot) UpdateSession(ctx context.Context, withPollToken bool, status string, lease *rbe.Lease) (*rbe.Lease, error) {
	if !b.HasSession() {
		return nil, errors.Reason("no healthy session").Err()
	}

	if lease != nil {
		logging.Infof(ctx, "Updating the session: %s [%s=%s]", status, lease.ID, lease.State)
	} else {
		logging.Infof(ctx, "Updating the session: %s", status)
	}

	var pollToken []byte
	if withPollToken {
		pollToken = b.pollToken
	}

	var resp rbe.UpdateBotSessionResponse
	err := b.rpc(ctx, "/swarming/api/v1/bot/rbe/session/update", &rbe.UpdateBotSessionRequest{
		SessionToken: b.sessionToken,
		PollToken:    pollToken,
		Dimensions:   b.dimensions,
		Status:       status,
		Lease:        lease,
	}, &resp)
	if err != nil {
		return nil, errors.Annotate(err, "updating the session").Err()
	}

	b.sessionToken = resp.SessionToken
	b.sessionStatus = resp.Status
	b.sessionExpiry = time.Unix(resp.SessionExpiry, 0).UTC()

	return resp.Lease, nil
}
