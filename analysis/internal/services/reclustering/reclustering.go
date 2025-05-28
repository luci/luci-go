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

package reclustering

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/analysis/internal/analysis"
	"go.chromium.org/luci/analysis/internal/analysis/clusteredfailures"
	"go.chromium.org/luci/analysis/internal/clustering/chunkstore"
	"go.chromium.org/luci/analysis/internal/clustering/reclustering"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
)

const (
	taskClass = "reclustering"
	queue     = "reclustering"
)

var tc = tq.RegisterTaskClass(tq.TaskClass{
	ID:        taskClass,
	Prototype: &taskspb.ReclusterChunks{},
	Queue:     queue,
	Kind:      tq.NonTransactional,
})

// RegisterTaskHandler registers the handler for reclustering tasks.
func RegisterTaskHandler(srv *server.Server) error {
	ctx := srv.Context
	cfg, err := config.Get(ctx)
	if err != nil {
		return err
	}
	chunkStore, err := chunkstore.NewClient(ctx, cfg.ChunkGcsBucket)
	if err != nil {
		return err
	}
	srv.RegisterCleanup(func(context.Context) {
		chunkStore.Close()
	})

	cf, err := clusteredfailures.NewClient(ctx, srv.Options.CloudProject)
	if err != nil {
		return err
	}
	srv.RegisterCleanup(func(context.Context) {
		cf.Close()
	})

	analysis := analysis.NewClusteringHandler(cf)
	worker := reclustering.NewWorker(chunkStore, analysis)

	handler := func(ctx context.Context, payload proto.Message) error {
		task := payload.(*taskspb.ReclusterChunks)
		return reclusterTestResults(ctx, worker, task)
	}
	tc.AttachHandler(handler)
	return nil
}

// Schedule enqueues a task to recluster a range of chunks in a LUCI
// Project.
func Schedule(ctx context.Context, task *taskspb.ReclusterChunks) error {
	title := fmt.Sprintf("%s-%s-shard-%v", task.Project, task.AttemptTime.AsTime().Format("20060102-150405"), task.EndChunkId)

	dedupKey, err := randomDeduplicationKey()
	if err != nil {
		return errors.Fmt("obtain deduplication key: %w", err)
	}
	taskProto := &tq.Task{
		Title: title,
		// Copy the task to avoid the caller retaining an alias to
		// the task proto passed to tq.AddTask.
		Payload: proto.Clone(task).(*taskspb.ReclusterChunks),
		// Use a deduplication key to avoid retried task creations
		// accidentally resulting in two tasks being created, in case
		// of failure to receive CreateTask response.
		// Note that this is only a best-effort deduplication, the
		// task should still assume the possibility of multiple
		// tasks being created and avoid data correctness issues
		// in this case.
		DeduplicationKey: dedupKey,
	}

	// After 50 seconds, task creation is probably pointless as
	// each reclustering run takes 1 minute.
	ctx, cancel := context.WithTimeout(ctx, 50*time.Second)
	defer cancel()

	// Manually retry transient errors. The Cloud Tasks client
	// does not automatically retry CreateTask RPCs, presumably
	// as the RPC does not offer strong guarantees against multiple
	// task creation in case of retry.
	err = retry.Retry(ctx, transient.Only(retry.Default), func() error {
		err := tq.AddTask(ctx, taskProto)
		if err != nil {
			return errors.Fmt("create task: %w", err)
		}
		return nil
	}, nil)
	return err
}

func randomDeduplicationKey() (string, error) {
	var b [16]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return "", errors.Fmt("read random bytes: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

func reclusterTestResults(ctx context.Context, worker *reclustering.Worker, task *taskspb.ReclusterChunks) error {
	next, err := worker.Do(ctx, task, reclustering.TargetTaskDuration)
	if err != nil {
		logging.Errorf(ctx, "Error re-clustering: %s", err)
		return err
	}
	if next != nil {
		if err := Schedule(ctx, next); err != nil {
			logging.Errorf(ctx, "Error scheduling continuation: %s", err)
			return err
		}
	}
	return nil
}
