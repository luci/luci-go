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

// Package swarmingtest contains Swarming client test helpers.
package swarmingtest

import (
	"context"
	"io"
	"time"

	"go.chromium.org/luci/swarming/client/swarming"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"
)

// Client is a mock of swarming.Client that just calls provided callbacks.
type Client struct {
	NewTaskMock        func(ctx context.Context, req *swarmingpb.NewTaskRequest) (*swarmingpb.TaskRequestMetadataResponse, error)
	CountTasksMock     func(ctx context.Context, start float64, state swarmingpb.StateQuery, tags []string) (*swarmingpb.TasksCount, error)
	ListTasksMock      func(ctx context.Context, limit int32, start float64, state swarmingpb.StateQuery, tags []string) ([]*swarmingpb.TaskResultResponse, error)
	CancelTaskMock     func(ctx context.Context, taskID string, killRunning bool) (*swarmingpb.CancelResponse, error)
	CancelTasksMock    func(ctx context.Context, limit int32, tags []string, killRunning bool, start, end time.Time) (*swarmingpb.TasksCancelResponse, error)
	TaskRequestMock    func(ctx context.Context, taskID string) (*swarmingpb.TaskRequestResponse, error)
	TaskOutputMock     func(ctx context.Context, taskID string, out io.Writer) (swarmingpb.TaskState, error)
	TaskResultMock     func(ctx context.Context, taskID string, fields *swarming.TaskResultFields) (*swarmingpb.TaskResultResponse, error)
	TaskResultsMock    func(ctx context.Context, taskIDs []string, fields *swarming.TaskResultFields) ([]swarming.ResultOrErr, error)
	ListTaskStatesMock func(ctx context.Context, taskIDs []string) ([]swarmingpb.TaskState, error)
	CountBotsMock      func(ctx context.Context, dimensions []*swarmingpb.StringPair) (*swarmingpb.BotsCount, error)
	ListBotsMock       func(ctx context.Context, dimensions []*swarmingpb.StringPair) ([]*swarmingpb.BotInfo, error)
	DeleteBotMock      func(ctx context.Context, botID string) (*swarmingpb.DeleteResponse, error)
	TerminateBotMock   func(ctx context.Context, botID string, reason string) (*swarmingpb.TerminateResponse, error)
	ListBotTasksMock   func(ctx context.Context, botID string, limit int32, start float64, state swarmingpb.StateQuery) ([]*swarmingpb.TaskResultResponse, error)
	FilesFromCASMock   func(ctx context.Context, outdir string, casRef *swarmingpb.CASReference) ([]string, error)
}

func (c *Client) Close(ctx context.Context) {}

func (c *Client) NewTask(ctx context.Context, req *swarmingpb.NewTaskRequest) (*swarmingpb.TaskRequestMetadataResponse, error) {
	return c.NewTaskMock(ctx, req)
}

func (c *Client) CountTasks(ctx context.Context, start float64, state swarmingpb.StateQuery, tags []string) (*swarmingpb.TasksCount, error) {
	return c.CountTasksMock(ctx, start, state, tags)
}

func (c *Client) ListTasks(ctx context.Context, limit int32, start float64, state swarmingpb.StateQuery, tags []string) ([]*swarmingpb.TaskResultResponse, error) {
	return c.ListTasksMock(ctx, limit, start, state, tags)
}

func (c *Client) CancelTask(ctx context.Context, taskID string, killRunning bool) (*swarmingpb.CancelResponse, error) {
	return c.CancelTaskMock(ctx, taskID, killRunning)
}

func (c *Client) CancelTasks(ctx context.Context, limit int32, tags []string, killRunning bool, start, end time.Time) (*swarmingpb.TasksCancelResponse, error) {
	return c.CancelTasksMock(ctx, limit, tags, killRunning, start, end)
}

func (c *Client) TaskRequest(ctx context.Context, taskID string) (*swarmingpb.TaskRequestResponse, error) {
	return c.TaskRequestMock(ctx, taskID)
}

func (c *Client) TaskOutput(ctx context.Context, taskID string, out io.Writer) (swarmingpb.TaskState, error) {
	return c.TaskOutputMock(ctx, taskID, out)
}

func (c *Client) TaskResult(ctx context.Context, taskID string, fields *swarming.TaskResultFields) (*swarmingpb.TaskResultResponse, error) {
	return c.TaskResultMock(ctx, taskID, fields)
}

func (c *Client) TaskResults(ctx context.Context, taskIDs []string, fields *swarming.TaskResultFields) ([]swarming.ResultOrErr, error) {
	return c.TaskResultsMock(ctx, taskIDs, fields)
}

func (c *Client) ListTaskStates(ctx context.Context, taskIDs []string) ([]swarmingpb.TaskState, error) {
	return c.ListTaskStatesMock(ctx, taskIDs)
}

func (c *Client) CountBots(ctx context.Context, dimensions []*swarmingpb.StringPair) (*swarmingpb.BotsCount, error) {
	return c.CountBotsMock(ctx, dimensions)
}

func (c *Client) ListBots(ctx context.Context, dimensions []*swarmingpb.StringPair) ([]*swarmingpb.BotInfo, error) {
	return c.ListBotsMock(ctx, dimensions)
}

func (c *Client) DeleteBot(ctx context.Context, botID string) (*swarmingpb.DeleteResponse, error) {
	return c.DeleteBotMock(ctx, botID)
}

func (c *Client) TerminateBot(ctx context.Context, botID string, reason string) (*swarmingpb.TerminateResponse, error) {
	return c.TerminateBotMock(ctx, botID, reason)
}

func (c *Client) ListBotTasks(ctx context.Context, botID string, limit int32, start float64, state swarmingpb.StateQuery) ([]*swarmingpb.TaskResultResponse, error) {
	return c.ListBotTasksMock(ctx, botID, limit, start, state)
}

func (c *Client) FilesFromCAS(ctx context.Context, outdir string, casRef *swarmingpb.CASReference) ([]string, error) {
	return c.FilesFromCASMock(ctx, outdir, casRef)
}
