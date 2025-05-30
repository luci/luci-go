// Copyright 2015 The LUCI Authors.
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

package ui

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	mc "go.chromium.org/luci/gae/service/memcache"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	api "go.chromium.org/luci/scheduler/api/scheduler/v1"
	"go.chromium.org/luci/scheduler/appengine/engine"
	"go.chromium.org/luci/scheduler/appengine/internal"
)

func jobPage(ctx *router.Context) {
	c, w, r, p := ctx.Request.Context(), ctx.Writer, ctx.Request, ctx.Params
	e := config(c).Engine

	projectID := p.ByName("ProjectID")
	jobName := p.ByName("JobName")
	cursor := r.URL.Query().Get("c")

	job := jobFromEngine(ctx, projectID, jobName)
	if job == nil {
		return
	}

	wg := sync.WaitGroup{}

	var triggers []*internal.Trigger
	var triErr error

	var triageLog *engine.JobTriageLog
	var triageLogErr error

	var invsActive []*engine.Invocation
	var invsActiveErr error
	var haveInvsActive bool

	// Triggers, triage log and active invocations are shown only on the first
	// page of the results.
	if cursor == "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			triggers, triErr = e.ListTriggers(c, job)
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			triageLog, triageLogErr = e.GetJobTriageLog(c, job)
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			haveInvsActive = true
			invsActive, _, invsActiveErr = e.ListInvocations(c, job, engine.ListInvocationsOpts{
				PageSize:   100000000, // ~ ∞, UI doesn't paginate active invocations
				ActiveOnly: true,
			})
		}()
	}

	// Historical invocations are shown on all pages.
	var invsLog []*engine.Invocation
	var nextCursor string
	var invsLogErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		invsLog, nextCursor, invsLogErr = e.ListInvocations(c, job, engine.ListInvocationsOpts{
			PageSize:     50,
			Cursor:       cursor,
			FinishedOnly: true,
		})
	}()

	wg.Wait()

	switch {
	case invsActiveErr != nil:
		panic(errors.Fmt("failed to fetch active invocations: %w", invsActiveErr))
	case invsLogErr != nil:
		panic(errors.Fmt("failed to fetch invocation log: %w", invsLogErr))
	case triErr != nil:
		panic(errors.Fmt("failed to fetch triggers: %w", triErr))
	case triageLogErr != nil:
		panic(errors.Fmt("failed to fetch triage log: %w", triageLogErr))
	}

	// memcacheKey hashes cursor to reduce its length, since full cursor doesn't
	// fit into memcache key length limits. Use 'v2' scheme for this ('v1' was
	// used before hashing was added).
	memcacheKey := func(cursor string) string {
		blob := sha256.Sum256([]byte(job.JobID + ":" + cursor))
		encoded := base64.StdEncoding.EncodeToString(blob[:])
		return "v2:cursors:list_invocations:" + encoded
	}

	// Cheesy way of implementing bidirectional pagination with forward-only
	// datastore cursors: store mapping from a page cursor to a previous page
	// cursor in the memcache.
	prevCursor := ""
	if cursor != "" {
		if itm, err := mc.GetKey(c, memcacheKey(cursor)); err == nil {
			prevCursor = string(itm.Value())
		}
	}
	if nextCursor != "" {
		itm := mc.NewItem(c, memcacheKey(nextCursor))
		if cursor == "" {
			itm.SetValue([]byte("NULL"))
		} else {
			itm.SetValue([]byte(cursor))
		}
		itm.SetExpiration(24 * time.Hour)
		mc.Set(c, itm)
	}

	// List of invocations in job.ActiveInvocations may contain recently finished
	// invocations not yet removed from the active list by the triage procedure.
	// 'invsActive' is always more accurate, since it fetches invocations from
	// the datastore and checks their status. So update the job entity to be more
	// accurate if we can. This is important for reporting jobs with recently
	// finished invocations as not running. Otherwise the UI page may appear
	// non-consistent (no running invocations in the list, yet the job's status is
	// displayed as "Running"). This is a bit of a hack...
	if haveInvsActive {
		ids := make([]int64, len(invsActive))
		for i, inv := range invsActive {
			ids[i] = inv.ID
		}
		job.ActiveInvocations = ids
	}

	jobUI := makeJob(c, job, triageLog)
	invsActiveUI := make([]*invocation, len(invsActive))
	for i, inv := range invsActive {
		invsActiveUI[i] = makeInvocation(jobUI, inv)
	}
	invsLogUI := make([]*invocation, len(invsLog))
	for i, inv := range invsLog {
		invsLogUI[i] = makeInvocation(jobUI, inv)
	}

	templates.MustRender(c, w, "pages/job.html", map[string]any{
		"Job":               jobUI,
		"ShowJobHeader":     cursor == "",
		"InvocationsActive": invsActiveUI,
		"InvocationsLog":    invsLogUI,
		"PendingTriggers":   makeTriggerList(jobUI.now, triggers),
		"PrevCursor":        prevCursor,
		"NextCursor":        nextCursor,
	})
}

////////////////////////////////////////////////////////////////////////////////
// Actions.

var errCannotTriggerPausedJob = errors.New("cannot trigger paused job")

func triggerJobAction(c *router.Context) {
	handleJobAction(c, func(job *engine.Job) error {
		ctx := c.Request.Context()
		eng := config(ctx).Engine

		// Paused jobs just silently ignore triggers. Warn the user.
		if job.Paused {
			return errCannotTriggerPausedJob
		}

		// Generate random ID for the trigger, since we need one. They are usually
		// used to guarantee idempotency, and thus should be provided by the
		// triggering side (which in this case is end-user's browser). We could
		// potentially generate the ID in Javascript and submit the trigger via URL
		// fetch,  and retry on transient errors until success, but this looks like
		// too much hassle for little gains.
		buf := make([]byte, 8)
		if _, err := rand.Read(buf); err != nil {
			return err
		}
		id := hex.EncodeToString(buf)

		// This will check the ACL and submit the trigger.
		return eng.EmitTriggers(ctx, map[*engine.Job][]*internal.Trigger{
			job: {
				{
					Id:            id,
					Created:       timestamppb.New(clock.Now(ctx)),
					Title:         "Triggered via web UI",
					EmittedByUser: string(auth.CurrentIdentity(ctx)),
					Payload: &internal.Trigger_Webui{
						Webui: &api.WebUITrigger{},
					},
				},
			},
		})
	})
}

func pauseJobAction(c *router.Context) {
	handleJobAction(c, func(job *engine.Job) error {
		reason := c.Request.PostForm.Get("reason")
		return config(c.Request.Context()).Engine.PauseJob(c.Request.Context(), job, reason)
	})
}

func resumeJobAction(c *router.Context) {
	handleJobAction(c, func(job *engine.Job) error {
		reason := c.Request.PostForm.Get("reason") // note: currently unset
		return config(c.Request.Context()).Engine.ResumeJob(c.Request.Context(), job, reason)
	})
}

func abortJobAction(c *router.Context) {
	handleJobAction(c, func(job *engine.Job) error {
		return config(c.Request.Context()).Engine.AbortJob(c.Request.Context(), job)
	})
}

func handleJobAction(c *router.Context, cb func(*engine.Job) error) {
	projectID := c.Params.ByName("ProjectID")
	jobName := c.Params.ByName("JobName")

	job := jobFromEngine(c, projectID, jobName)
	if job == nil {
		return
	}

	switch err := cb(job); {
	case err == errCannotTriggerPausedJob:
		uiErrCannotTriggerPausedJob.render(c)
	case err == engine.ErrNoPermission:
		uiErrActionForbidden.render(c)
	case err != nil:
		panic(err)
	default:
		http.Redirect(c.Writer, c.Request, fmt.Sprintf("/jobs/%s/%s", projectID, jobName), http.StatusFound)
	}
}
