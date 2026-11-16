// Copyright 2026 The LUCI Authors.
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
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/user"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/xsrf"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/scheduler/appengine/catalog"
	"go.chromium.org/luci/scheduler/appengine/engine"
)

func TestUIHandlers(t *testing.T) {
	t.Parallel()

	ftt.Run("UI handlers", t, func(t *ftt.Test) {
		ctx := gaetesting.TestingContext()
		ctx = auth.ModifyConfig(ctx, func(cfg auth.Config) auth.Config {
			cfg.AnonymousTransport = func(context.Context) http.RoundTripper {
				return http.DefaultTransport
			}
			cfg.DBProvider = func(context.Context) (authdb.DB, error) {
				return authtest.NewFakeDB(), nil
			}
			return cfg
		})

		eng := &fakeEngine{
			getVisibleJobs: func() ([]*engine.Job, error) {
				return nil, nil
			},
			getVisibleProjectJobs: func(string) ([]*engine.Job, error) {
				return nil, nil
			},
			getVisibleJob: func(string) (*engine.Job, error) {
				return nil, engine.ErrNoSuchJob
			},
			getInvocation: func(*engine.Job, int64) (*engine.Invocation, error) {
				return nil, engine.ErrNoSuchInvocation
			},
		}

		r := router.New()
		base := router.NewMiddlewareChain(func(c *router.Context, next router.Handler) {
			c.Request = c.Request.WithContext(ctx)
			next(c)
		})
		InstallHandlers(r, base, Config{
			Engine:        eng,
			Catalog:       catalog.New(),
			TemplatesPath: filepath.Join("..", "frontend", "templates"),
		})

		serve := func(method, target string) *httptest.ResponseRecorder {
			req := httptest.NewRequest(method, target, nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			return rec
		}

		t.Run("Anonymous GET on empty index renders without redirect", func(t *ftt.Test) {
			rec := serve(http.MethodGet, "/")
			assert.That(t, rec.Code, should.Equal(http.StatusOK))
		})

		t.Run("Anonymous GET on empty or restricted project redirects to login", func(t *ftt.Test) {
			rec := serve(http.MethodGet, "/jobs/chrome?filter=running")
			assert.That(t, rec.Code, should.Equal(http.StatusFound))
			assert.That(t, rec.Header().Get("Location"), should.Equal("https://fakeapp.example.com/_ah/login?redirect=%2Fjobs%2Fchrome%3Ffilter%3Drunning"))
			assert.That(t, rec.Body.String(), should.NotContainSubstring("<!DOCTYPE html>"))
		})

		t.Run("Authenticated GET on empty or restricted project renders empty page without redirect", func(t *ftt.Test) {
			user.GetTestable(ctx).Login("user@example.com", "", false)
			rec := serve(http.MethodGet, "/jobs/chrome")
			assert.That(t, rec.Code, should.Equal(http.StatusOK))
			assert.That(t, rec.Body.String(), should.ContainSubstring("No jobs in this project or no READER permission to view them."))
			assert.That(t, rec.Body.String(), should.ContainSubstring(`href="https://fakeapp.example.com/_ah/logout?redirect=%2F"`))
		})

		t.Run("Anonymous GET on project with public jobs renders without redirect", func(t *ftt.Test) {
			eng.getVisibleProjectJobs = func(projectID string) ([]*engine.Job, error) {
				return []*engine.Job{
					{
						JobID:     projectID + "/pub-job",
						ProjectID: projectID,
						Enabled:   true,
						Schedule:  "with 10m interval",
					},
				}, nil
			}
			rec := serve(http.MethodGet, "/jobs/chromium")
			assert.That(t, rec.Code, should.Equal(http.StatusOK))
			assert.That(t, rec.Body.String(), should.ContainSubstring("pub-job"))

			rec = serve(http.MethodGet, "/jobs/chromium?filter=running")
			assert.That(t, rec.Code, should.Equal(http.StatusOK))
			assert.That(t, rec.Body.String(), should.ContainSubstring("No running jobs"))
			assert.That(t, rec.Body.String(), should.ContainSubstring(`href="https://fakeapp.example.com/_ah/login?redirect=%2Fjobs%2Fchromium%3Ffilter%3Drunning"`))
		})

		t.Run("Anonymous GET on missing or restricted job redirects to login", func(t *ftt.Test) {
			rec := serve(http.MethodGet, "/jobs/chrome/secret-job?c=cursor1")
			assert.That(t, rec.Code, should.Equal(http.StatusFound))
			assert.That(t, rec.Header().Get("Location"), should.Equal("https://fakeapp.example.com/_ah/login?redirect=%2Fjobs%2Fchrome%2Fsecret-job%3Fc%3Dcursor1"))
			assert.That(t, rec.Body.String(), should.NotContainSubstring("<!DOCTYPE html>"))
		})

		t.Run("Anonymous GET on missing or restricted invocation redirects to login", func(t *ftt.Test) {
			rec := serve(http.MethodGet, "/jobs/chrome/secret-job/12345")
			assert.That(t, rec.Code, should.Equal(http.StatusFound))
			assert.That(t, rec.Header().Get("Location"), should.Equal("https://fakeapp.example.com/_ah/login?redirect=%2Fjobs%2Fchrome%2Fsecret-job%2F12345"))
			assert.That(t, rec.Body.String(), should.NotContainSubstring("<!DOCTYPE html>"))
		})

		t.Run("Anonymous GET on missing invocation of public job returns 404 without redirect", func(t *ftt.Test) {
			eng.getVisibleJob = func(jobID string) (*engine.Job, error) {
				return &engine.Job{
					JobID:     jobID,
					ProjectID: "chromium",
					Enabled:   true,
					Schedule:  "with 10m interval",
				}, nil
			}
			rec := serve(http.MethodGet, "/jobs/chromium/pub-job/99999")
			assert.That(t, rec.Code, should.Equal(http.StatusNotFound))
			assert.That(t, rec.Body.String(), should.ContainSubstring("The requested invocation is not found."))
		})

		t.Run("Anonymous POST action on missing or restricted job returns 404 without redirect", func(t *ftt.Test) {
			tok, err := xsrf.Token(auth.WithState(ctx, &authtest.FakeState{}))
			assert.NoErr(t, err)
			form := url.Values{"xsrf_token": {tok}}
			req := httptest.NewRequest(http.MethodPost, "/actions/triggerJob/chrome/secret-job", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			assert.That(t, rec.Code, should.Equal(http.StatusNotFound))
			assert.That(t, rec.Body.String(), should.ContainSubstring("No such job or no permission to view it."))
			assert.That(t, rec.Body.String(), should.ContainSubstring(`href="https://fakeapp.example.com/_ah/login?redirect=%2F"`))
		})

		t.Run("Anonymous POST action on missing or restricted invocation returns 404 without redirect", func(t *ftt.Test) {
			tok, err := xsrf.Token(auth.WithState(ctx, &authtest.FakeState{}))
			assert.NoErr(t, err)
			form := url.Values{"xsrf_token": {tok}}
			req := httptest.NewRequest(http.MethodPost, "/actions/abortInvocation/chrome/secret-job/12345", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			assert.That(t, rec.Code, should.Equal(http.StatusNotFound))
			assert.That(t, rec.Body.String(), should.ContainSubstring("No such job or no permission to view it."))
			assert.That(t, rec.Body.String(), should.ContainSubstring(`href="https://fakeapp.example.com/_ah/login?redirect=%2F"`))
		})

		t.Run("Authenticated GET on missing or restricted job returns 404 without redirect", func(t *ftt.Test) {
			user.GetTestable(ctx).Login("user@example.com", "", false)
			rec := serve(http.MethodGet, "/jobs/chrome/secret-job")
			assert.That(t, rec.Code, should.Equal(http.StatusNotFound))
			assert.That(t, rec.Body.String(), should.ContainSubstring("No such job or no permission to view it."))
		})

		t.Run("Authenticated GET on missing or restricted invocation returns 404 without redirect", func(t *ftt.Test) {
			user.GetTestable(ctx).Login("user@example.com", "", false)
			rec := serve(http.MethodGet, "/jobs/chrome/secret-job/12345")
			assert.That(t, rec.Code, should.Equal(http.StatusNotFound))
			assert.That(t, rec.Body.String(), should.ContainSubstring("No such job or no permission to view it."))
		})
	})
}

type fakeEngine struct {
	engine.Engine
	getVisibleJobs        func() ([]*engine.Job, error)
	getVisibleProjectJobs func(projectID string) ([]*engine.Job, error)
	getVisibleJob         func(jobID string) (*engine.Job, error)
	getInvocation         func(job *engine.Job, invID int64) (*engine.Invocation, error)
}

func (f *fakeEngine) GetVisibleJobs(_ context.Context) ([]*engine.Job, error) {
	return f.getVisibleJobs()
}

func (f *fakeEngine) GetVisibleProjectJobs(_ context.Context, projectID string) ([]*engine.Job, error) {
	return f.getVisibleProjectJobs(projectID)
}

func (f *fakeEngine) GetVisibleJob(_ context.Context, jobID string) (*engine.Job, error) {
	return f.getVisibleJob(jobID)
}

func (f *fakeEngine) GetInvocation(_ context.Context, job *engine.Job, invID int64) (*engine.Invocation, error) {
	return f.getInvocation(job, invID)
}
