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

package cfgcache

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/config"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/xsrf"
	cfgcachecommon "go.chromium.org/luci/server/cfgcache/common"
	cfgcachepb "go.chromium.org/luci/server/cfgcache/proto"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/router"

	_ "embed"
)

//go:embed status_page.html
var statusPageHTML string

func humanBytes(size int64) string {
	return humanize.Bytes(uint64(max(0, size)))
}

var statusPageTmpl = template.Must(template.New("status_page.html").Funcs(template.FuncMap{
	"humanBytes": humanBytes,
	"shortHash": func(h string) string {
		if len(h) > 12 {
			return h[:12]
		}
		if h == "" {
			return "-"
		}
		return h
	},
	"formatTime": func(t time.Time) string {
		if t.IsZero() {
			return "never"
		}
		return t.UTC().Format(time.RFC3339)
	},
	"configURL": func(pipelineID string, cs config.Set) string {
		q := url.Values{
			"pipeline":   {pipelineID},
			"config_set": {string(cs)},
		}
		return StatusConfigPath + "?" + q.Encode()
	},
}).Parse(statusPageHTML))

type statusPageData struct {
	StatusPagePath   string
	StatusIngestPath string
	StatusConfigPath string

	Identity       string
	LogoutURL      string
	XsrfTokenField template.HTML
	AdminGroup     string

	Now                  time.Time
	InMemoryEnabled      bool
	CronEnabled          bool
	ValidationEnabled    bool
	MemoryRefreshTimeout time.Duration
	MaxIngestMemoryMB    int64

	IngestResult *ingestActionResult
	Pipelines    []pipelineStatus
}

type ingestActionResult struct {
	Duration time.Duration
	Err      string
}

type pipelineStatus struct {
	ID   string
	Desc string

	IsService          bool
	ServiceName        string
	ServiceNamePattern string

	// IsGettable reports whether the pipeline's Store implements GettableStore
	// (storeAdapter.gettable != nil), allowing stored configs to be fetched and
	// rendered as JSONPB on the status page.
	//
	// Relationship with HasMemoryRefresh:
	//   - HasMemoryRefresh (ic.toMemory != nil) requires a GettableStore during
	//     registration, so HasMemoryRefresh == true always implies
	//     IsGettable == true.
	//   - NoMemoryRefresh pipelines (HasMemoryRefresh == false) only require
	//     Store. IsGettable is still true when backed by a GettableStore (such
	//     as dsstore.Store or memstore.Store), and false for write-only Store
	//     implementations.
	IsGettable bool

	// HasMemoryRefresh reports whether in-memory refresh is enabled for this
	// pipeline (InMemory is not NoMemoryRefresh && ic.toMemory != nil).
	HasMemoryRefresh bool

	TransformVersion   int64
	ToMemoryMinVersion int64
	ToMemoryMaxVersion int64

	Files []fileStatus

	StoreErr           string
	LastRefreshed      time.Time
	LastRefreshAttempt time.Time
	LastRefreshErr     string

	OverallStatus string
	ConfigSets    []configSetStatus
}

type fileStatus struct {
	Pattern      string
	ResolvedPath string
	IsRegex      bool
}

type configSetStatus struct {
	ConfigSet    config.Set
	SyncStatus   string
	StoreMeta    *cfgcachepb.Meta
	AcceptedMeta *cfgcachepb.Meta
	Error        string
}

func (m *serverModule) installStatusHandlers(host module.Host, hostOpts module.HostOptions) {
	routes := host.Routes()
	if routes == nil {
		return
	}

	mw := router.NewMiddlewareChain(
		auth.Authenticate(host.CookieAuth()),
		m.authorizeStatusMiddleware(hostOpts.Prod),
	)

	routes.GET(StatusPagePath, mw, func(c *router.Context) {
		m.renderStatusPage(c, nil)
	})
	routes.GET(StatusConfigPath, mw, func(c *router.Context) {
		m.handleGetStoredConfig(c)
	})
	routes.POST(StatusIngestPath, mw.Extend(xsrf.WithTokenCheck), func(c *router.Context) {
		reg := m.registry()
		ctx := c.Request.Context()
		start := clock.Now(ctx)
		err := reg.IngestToStorage(ctx)
		res := &ingestActionResult{
			Duration: clock.Now(ctx).Sub(start).Round(time.Millisecond),
		}
		if err != nil {
			res.Err = err.Error()
		}
		err = reg.RefreshInMemory(ctx)
		if err != nil {
			res.Err = err.Error()
		}
		m.renderStatusPage(c, res)
	})
}

func (m *serverModule) authorizeStatusMiddleware(prod bool) router.Middleware {
	adminGroup := m.opts.StatusAdminGroup
	return func(c *router.Context, next router.Handler) {
		if !prod {
			next(c)
			return
		}

		ctx := c.Request.Context()
		u := auth.CurrentUser(ctx)
		if u.Superuser {
			next(c)
			return
		}

		if u.Identity == identity.AnonymousIdentity {
			destURL := *c.Request.URL
			destURL.Host = ""
			destURL.Scheme = ""
			loginURL, err := auth.LoginURL(ctx, destURL.String())
			switch {
			case err == nil:
				http.Redirect(c.Writer, c.Request, loginURL, http.StatusFound)
			case errors.Is(err, auth.ErrNoUsersAPI):
				http.Error(c.Writer, "Authentication required", http.StatusUnauthorized)
			case transient.Tag.In(err):
				logging.WithError(err).Errorf(ctx, "cfgcache status: transient error generating login URL")
				http.Error(c.Writer, "Transient error generating login URL", http.StatusInternalServerError)
			default:
				logging.WithError(err).Errorf(ctx, "cfgcache status: failed to generate login URL")
				http.Error(c.Writer, "Authentication required", http.StatusUnauthorized)
			}
			return
		}

		switch ok, err := auth.IsMember(ctx, adminGroup); {
		case err != nil:
			logging.WithError(err).Errorf(ctx, "cfgcache status: failed to check group membership in %q", adminGroup)
			http.Error(c.Writer, "Internal server error checking group membership", http.StatusInternalServerError)
		case !ok:
			http.Error(c.Writer, fmt.Sprintf("Access denied: %q is not a member of %q", u.Identity, adminGroup), http.StatusForbidden)
		default:
			next(c)
		}
	}
}

func sanitizeHeaderValue(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(s)
}

func (m *serverModule) handleGetStoredConfig(c *router.Context) {
	ctx := c.Request.Context()
	q := c.Request.URL.Query()
	pipelineID := q.Get("pipeline")
	cs := config.Set(q.Get("config_set"))
	if pipelineID == "" || cs == "" {
		http.Error(c.Writer, "Missing required query parameters: pipeline and config_set", http.StatusBadRequest)
		return
	}

	reg := m.registry()
	ic, ok := reg.ingestConfigs[pipelineID]
	if !ok {
		http.Error(c.Writer, fmt.Sprintf("Unknown pipeline %q", pipelineID), http.StatusNotFound)
		return
	}
	if !ic.store.isGettable() {
		http.Error(c.Writer, fmt.Sprintf("Pipeline %q store does not implement GettableStore", pipelineID), http.StatusBadRequest)
		return
	}

	var found *cfgcachecommon.StoredConfig[proto.Message]
	for sc, err := range ic.store.Get(ctx, ic.id, map[config.Set]*cfgcachepb.Meta{cs: {}}) {
		if err != nil {
			logging.WithError(err).Errorf(ctx, "cfgcache status: failed to load stored config %s[%s]", ic.id, cs)
			http.Error(c.Writer, fmt.Sprintf("Failed to load stored config: %s", err), http.StatusInternalServerError)
			return
		}
		if sc != nil && sc.ConfigSet == cs {
			found = sc
		}
	}
	if found == nil || found.Config == nil {
		http.Error(c.Writer, fmt.Sprintf("Config set %q not found in store for pipeline %q", cs, pipelineID), http.StatusNotFound)
		return
	}

	rawJSON, err := (protojson.MarshalOptions{
		UseProtoNames: q.Get("proto_names") == "1",
	}).Marshal(found.Config)
	if err != nil {
		logging.WithError(err).Errorf(ctx, "cfgcache status: failed to marshal %s[%s] as JSONPB", ic.id, cs)
		http.Error(c.Writer, fmt.Sprintf("Failed to marshal config as JSONPB: %s", err), http.StatusInternalServerError)
		return
	}
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, rawJSON, "", "  "); err != nil {
		logging.WithError(err).Errorf(ctx, "cfgcache status: failed to indent %s[%s] JSONPB", ic.id, cs)
		http.Error(c.Writer, "Failed to format JSONPB", http.StatusInternalServerError)
		return
	}
	prettyJSON.WriteByte('\n')

	hdr := c.Writer.Header()
	hdr.Set("Content-Type", "application/json; charset=utf-8")
	hdr.Set("X-Content-Type-Options", "nosniff")
	hdr.Set("Cache-Control", "no-store")
	hdr.Set("X-Cfgcache-Proto-Type", string(found.Config.ProtoReflect().Descriptor().FullName()))
	if found.Meta != nil {
		hdr.Set("X-Cfgcache-Input-Revision", sanitizeHeaderValue(found.Meta.InputRevision))
		hdr.Set("X-Cfgcache-Input-Hash", sanitizeHeaderValue(found.Meta.InputHash))
		hdr.Set("X-Cfgcache-Input-Size", strconv.FormatInt(found.Meta.InputSize, 10))
		hdr.Set("X-Cfgcache-Transformed-Size", strconv.FormatInt(found.Meta.TransformedSize, 10))
		hdr.Set("X-Cfgcache-Transformed-Size-Human", humanBytes(found.Meta.TransformedSize))
		hdr.Set("X-Cfgcache-Transform-Version", strconv.FormatInt(found.Meta.TransformVersion, 10))
	}
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write(prettyJSON.Bytes())
}

func (m *serverModule) renderStatusPage(c *router.Context, ingestRes *ingestActionResult) {
	ctx := c.Request.Context()
	reg := m.registry()

	logoutURL, err := auth.LogoutURL(ctx, "/")
	if err != nil && !errors.Is(err, auth.ErrNoUsersAPI) {
		logging.WithError(err).Warningf(ctx, "cfgcache status: failed to generate logout URL")
	}

	ids := slices.Sorted(maps.Keys(reg.ingestConfigs))
	pipelines := make([]pipelineStatus, 0, len(ids))
	for _, id := range ids {
		pipelines = append(pipelines, collectPipelineStatus(ctx, reg.ingestConfigs[id]))
	}

	data := statusPageData{
		StatusPagePath:       StatusPagePath,
		StatusIngestPath:     StatusIngestPath,
		StatusConfigPath:     StatusConfigPath,
		Identity:             string(auth.CurrentUser(ctx).Identity),
		LogoutURL:            logoutURL,
		XsrfTokenField:       xsrf.TokenField(ctx),
		Now:                  clock.Now(ctx).UTC(),
		AdminGroup:           m.opts.StatusAdminGroup,
		InMemoryEnabled:      !m.opts.DisableInMemory,
		CronEnabled:          !m.opts.DisableCron,
		ValidationEnabled:    !m.opts.DisableValidation,
		MemoryRefreshTimeout: m.opts.MemoryRefreshTimeout,
		MaxIngestMemoryMB:    reg.MaxIngestMemoryMB,
		IngestResult:         ingestRes,
		Pipelines:            pipelines,
	}

	var buf bytes.Buffer
	if err := statusPageTmpl.Execute(&buf, data); err != nil {
		logging.WithError(err).Errorf(ctx, "cfgcache status: failed to render template")
		http.Error(c.Writer, "Internal server error rendering status page", http.StatusInternalServerError)
		return
	}
	c.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write(buf.Bytes())
}

func collectPipelineStatus(ctx context.Context, ic *ingestConfig) pipelineStatus {
	hasMemoryRefresh := ic.toMemory != nil

	snap := ic.state.snapshot()
	dbState, storeErr := ic.store.LoadMeta(ctx, ic.id)

	files := make([]fileStatus, len(ic.files))
	for i, rf := range ic.files {
		files[i] = fileStatus{
			Pattern:      rf.pattern(),
			ResolvedPath: rf.path,
			IsRegex:      rf.isRegex(),
		}
	}

	ps := pipelineStatus{
		ID:                 ic.id,
		Desc:               ic.desc,
		IsService:          ic.serviceName != "",
		ServiceName:        ic.serviceName,
		ServiceNamePattern: ic.serviceNamePattern,
		TransformVersion:   ic.transformVersion,
		IsGettable:         ic.store.isGettable(),
		HasMemoryRefresh:   hasMemoryRefresh,
		ToMemoryMinVersion: ic.toMemoryMinVersion,
		ToMemoryMaxVersion: ic.toMemoryMaxVersion,
		Files:              files,
		LastRefreshed:      snap.lastRefreshed,
		LastRefreshAttempt: snap.lastRefreshAttempt,
	}
	if storeErr != nil {
		ps.StoreErr = storeErr.Error()
	}
	if snap.lastRefreshErr != nil {
		ps.LastRefreshErr = snap.lastRefreshErr.Error()
	}

	allSets := make(map[config.Set]struct{}, len(dbState)+len(snap.acceptedState)+len(snap.configSetErrors))
	for cs := range dbState {
		allSets[cs] = struct{}{}
	}
	for cs := range snap.acceptedState {
		allSets[cs] = struct{}{}
	}
	for cs := range snap.configSetErrors {
		allSets[cs] = struct{}{}
	}

	hasRowErr := false
	for _, cs := range slices.Sorted(maps.Keys(allSets)) {
		storeMeta := dbState[cs]
		acceptedMeta := snap.acceptedState[cs]
		csErr := snap.configSetErrors[cs]

		row := configSetStatus{
			ConfigSet:    cs,
			StoreMeta:    storeMeta,
			AcceptedMeta: acceptedMeta,
		}
		if csErr != nil {
			row.Error = csErr.Error()
		}

		switch {
		case !hasMemoryRefresh:
			row.SyncStatus = "STORE_ONLY"
		case csErr != nil:
			hasRowErr = true
			if storeMeta != nil && (storeMeta.TransformVersion < ic.toMemoryMinVersion || storeMeta.TransformVersion > ic.toMemoryMaxVersion) {
				row.SyncStatus = "VERSION_MISMATCH"
			} else {
				row.SyncStatus = "TO_MEMORY_ERROR"
			}
		case storeMeta != nil && acceptedMeta != nil && proto.Equal(storeMeta, acceptedMeta):
			row.SyncStatus = "OK"
		default:
			hasRowErr = true
			row.SyncStatus = "STALE"
		}

		ps.ConfigSets = append(ps.ConfigSets, row)
	}

	switch {
	case !hasMemoryRefresh:
		if storeErr != nil {
			ps.OverallStatus = "ERROR"
		} else {
			ps.OverallStatus = "STORE_ONLY"
		}
	case storeErr != nil || snap.lastRefreshErr != nil || hasRowErr:
		ps.OverallStatus = "ERROR"
	default:
		ps.OverallStatus = "HEALTHY"
	}

	return ps
}
