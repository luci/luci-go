// Copyright 2016 The LUCI Authors.
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

package projectconfig

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	protoutil "go.chromium.org/luci/common/proto"
	luciconfigpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	configInterface "go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/milo/internal/utils"
	projectconfigpb "go.chromium.org/luci/milo/proto/projectconfig"
)

// Project is a datastore entity representing a single project.
//
// Its children are consoles. This entity exists even if the project doesn't
// define milo.cfg config file. It has HasConfig == false in this case. We
// still need the entity to be able to check project's ACLs when accessing
// individual build pages.
type Project struct {
	ID        string `gae:"$id"`
	HasConfig bool
	ACL       ACL `gae:",noindex"`

	LogoURL        string
	BugURLTemplate string

	// IgnoredBuilderIds is a list of builder IDs to be ignored by pubsub handler.
	// ID format: <bucket>/<builder>
	IgnoredBuilderIDs []string

	// ExternalBuilderIDs is a list of builder IDs that are not in this project
	// but are referenced by this project.
	// ID format: <project>/<bucket>/<builder>
	ExternalBuilderIDs []string

	MetadataConfig []byte `gae:",noindex"`

	// Tolerate unknown fields when fetching entities.
	_ datastore.PropertyMap `gae:"-,extra"`
}

// BuilderIsIgnored checks if the builder is marked as ignored in this project.
func (p *Project) BuilderIsIgnored(builderID *buildbucketpb.BuilderID) bool {
	builderIDStr := fmt.Sprintf("%s/%s", builderID.Bucket, builderID.Builder)
	for _, bid := range p.IgnoredBuilderIDs {
		if bid == builderIDStr {
			return true
		}
	}
	return false
}

// ACL lists groups and identities that are allowed to see consoles in
// a project.
//
// Fetched from project.cfg config file in fetchProjectACL.
type ACL struct {
	Groups     []string
	Identities []identity.Identity
}

// Console is a datastore entity representing a single console.
type Console struct {
	// Parent is a key to the parent Project entity where this console was
	// defined in.
	Parent *datastore.Key `gae:"$parent"`

	// ID is the ID of the console.
	// This ID is local to the console, i.e. it should equal .Def.Id and not
	// Def.ExternalId.
	ID string `gae:"$id"`

	// Ordinal specifies the console's ordering in its project's consoles list.
	Ordinal int

	// The URL to the luci-config definition of this console.
	ConfigURL string

	// The luci-config revision from when this Console was retrieved.
	ConfigRevision string `gae:",noindex"`

	// (indexed) All builder IDs mentioned by this console config.
	Builders []string

	// Def is the actual underlying proto Console definition.
	// If this console is external (i.e. a reference to a console from
	// another project), this will contain the resolved Console definition,
	// but with ExternalId and ExternalProject also set.
	Def *projectconfigpb.Console `gae:",legacy"`

	// Realm that the console exists under.
	Realm string

	// _ is a "black hole" which absorbs any extra props found during a
	// datastore Get. These props are not written back on a datastore Put.
	_ datastore.PropertyMap `gae:"-,extra"`
}

// IsExternal returns whether the console is a reference to a console from
// another project. If this is the case, the console will have Def.ExternalProject
// and Def.ExternalId set.
func (c *Console) IsExternal() bool {
	return c.Def.GetExternalProject() != ""
}

// ConsoleID returns a struct containing ID strings for the console and its parent project.
func (c *Console) ConsoleID() ConsoleID {
	return ConsoleID{Project: c.ProjectID(), ID: c.ID}
}

// ProjectID retrieves the project ID string of the console out of the Console's
// parent key.
func (c *Console) ProjectID() string {
	if c.Parent == nil {
		return ""
	}
	return c.Parent.StringID()
}

// FilterBuilders filters out builder IDs and builders with realms not listed
// in `allowedRealms`.
// Deprecated: use `projectconfigpb.Console.AllowedBuilders` instead. Favor
// functional code over modifying the object in place.
func (c *Console) FilterBuilders(allowedRealms stringset.Set) {
	okBuilderIDs := make([]string, 0, len(c.Builders))
	for _, id := range c.Builders {
		bid, err := utils.ParseLegacyBuilderID(id)
		if err != nil {
			// The config is validated when ingested by milo, so this should never
			// happen.
			panic(err)
		}
		if !allowedRealms.Has(realms.Join(bid.Project, bid.Bucket)) {
			continue
		}
		okBuilderIDs = append(okBuilderIDs, id)
	}
	c.Builders = okBuilderIDs
	okBuilders := make([]*projectconfigpb.Builder, 0, len(c.Def.Builders))
	for _, b := range c.Def.Builders {
		if !allowedRealms.Has(realms.Join(b.Id.Project, b.Id.Bucket)) {
			continue
		}
		okBuilders = append(okBuilders, b)
	}
	c.Def.Builders = okBuilders
}

// BuilderRealms returns all realms referenced by this Console's Builders.
func (c *Console) BuilderRealms() stringset.Set {
	builderRealms := stringset.New(1)
	for _, id := range c.Builders {
		bid, err := utils.ParseLegacyBuilderID(id)
		if err != nil {
			// The config is validated when ingested by milo, so this should never
			// happen.
			panic(err)
		}
		builderRealms.Add(realms.Join(bid.Project, bid.Bucket))
	}
	return builderRealms
}

// ConsoleID is a reference to a console.
type ConsoleID struct {
	Project string
	ID      string
}

// ParseConsoleID reformats the string into a ConsoleID.
func ParseConsoleID(id string) (cid ConsoleID, err error) {
	components := strings.Split(id, "/")
	if len(components) != 2 {
		err = errors.New("invalid console id: " + id)
		return
	}
	return ConsoleID{
		Project: components[0],
		ID:      components[1],
	}, nil
}

func (id *ConsoleID) String() string {
	return fmt.Sprintf("%s/%s", id.Project, id.ID)
}

// SetID returns an empty Console datastore entity keyed with itself.
func (id *ConsoleID) SetID(c context.Context, console *Console) *Console {
	if console == nil {
		console = &Console{}
	}
	console.Parent = datastore.MakeKey(c, "Project", id.Project)
	console.ID = id.ID
	return console
}

// ErrConsoleNotFound is returned from GetConsole if the requested console
// isn't known to exist.
var ErrConsoleNotFound = errors.New("console not found")

func init() {
	// We need to wait until error codes have generated tags before initializing this error
	ErrConsoleNotFound = grpcutil.NotFoundTag.Apply(

		// configURL returns a user friendly URL that specifies where to view
		// this console definition.
		errors.New("console not found"))
}

func configURL(c context.Context, meta *configInterface.Meta) string {
	// TODO(hinoka): This shouldn't be hardcoded, instead we should get the
	// luci-config instance from the context.  But we only use this instance at
	// the moment so it is okay for now.
	// TODO(hinoka): The UI doesn't allow specifying paths and revision yet.  Add
	// that in when it is supported.
	return fmt.Sprintf("https://luci-config.appspot.com/#/%s", meta.ConfigSet)
}

// ServiceConfigID is the key for the service config entity in datastore.
const ServiceConfigID = "service_config"

// ServiceConfig is a container for the instance's service config.
type ServiceConfig struct {
	// ID is the datastore key.  This should be static, as there should only be
	// one service config.
	ID string `gae:"$id"`
	// Revision is the revision of the config, taken from luci-config.  This is used
	// to determine if the entry needs to be refreshed.
	Revision string
	// Data is the binary proto of the config.
	Data []byte `gae:",noindex"`
	// Text is the text format of the config.  For human consumption only.
	Text string `gae:",noindex"`
	// LastUpdated is the time this config was last updated.
	LastUpdated time.Time
}

// fetchProject fetches the config for a single project and parses it.
//
// As an input takes project.cfg config (**not** milo.cfg) with ACLs. Fetches
// Milo's config from the same config set. Absence of the Milo config is not an
// error.
//
// Returns, in order:
//   - A Project entity to store in Datastore
//   - The parsed Milo config (may be nil if there was no milo.cfg)
//   - Metadata for the Milo config (may be nil if there was no milo.cfg)
func fetchProject(c context.Context, cfg *configInterface.Config) (*Project, *projectconfigpb.Project, *configInterface.Meta, error) {
	logging.Infof(c, "fetching configs from %q", cfg.ConfigSet)

	projectID := cfg.ConfigSet.Project()
	if projectID == "" {
		return nil, nil, nil, errors.Fmt("bad config set name %q, not a project", cfg.ConfigSet)
	}

	// Deserialize project.cfg and grab an ACL from it.
	acl, err := parseProjectACL(cfg.Content)
	if err != nil {
		return nil, nil, nil, errors.Fmt("parsing project.cfg: %w", err)
	}

	// The future project entity.
	project := Project{
		ID:  projectID,
		ACL: acl,
	}

	// Load the Milo config (if it exists).
	miloCfg := projectconfigpb.Project{}
	miloCfgMeta := configInterface.Meta{}
	err = cfgclient.Get(c,
		cfg.ConfigSet,
		"${appid}.cfg",
		cfgclient.ProtoText(&miloCfg),
		&miloCfgMeta,
	)
	switch {
	case err == configInterface.ErrNoConfig:
		// No Milo config. This is fine, just save Project entity.
		return &project, nil, nil, nil
	case err != nil:
		// Something blew up.
		return nil, nil, nil, errors.Fmt("when loading Milo config: %w", err)
	}

	// Backfill `id` if it's not defined.
	// TODO(crbug.com/1263768): remove this once `id` field is always populated
	// by lucicfg.
	for _, c := range miloCfg.Consoles {
		for _, b := range c.Builders {
			if b.Id != nil {
				continue
			}
			bid, err := utils.ParseLegacyBuilderID(b.Name)
			if err != nil {
				return nil, nil, nil, errors.Fmt("when backfilling builder ID: %w", err)
			}
			b.Id = bid
		}
	}

	// Have the Milo config! Use it to update the Project entity.
	project.HasConfig = true
	project.LogoURL = miloCfg.LogoUrl
	project.IgnoredBuilderIDs = miloCfg.IgnoredBuilderIds
	project.BugURLTemplate = miloCfg.BugUrlTemplate
	project.MetadataConfig, err = proto.Marshal(miloCfg.MetadataConfig)
	if err != nil {
		return nil, nil, nil, errors.Fmt("marshal metadataConfig: %w", err)
	}
	// Populate project.ExternalBuilderIDs
	for _, console := range miloCfg.Consoles {
		for _, builder := range console.Builders {
			bid := builder.Id
			if bid.Project != project.ID {
				project.ExternalBuilderIDs = append(project.ExternalBuilderIDs, bid.Project+"/"+bid.Bucket+"/"+bid.Builder)
			}
		}
	}
	sort.Strings(project.ExternalBuilderIDs)

	return &project, &miloCfg, &miloCfgMeta, nil
}

// prepareConsolesUpdate is called in a transaction to evaluate which Console
// entities should be updated for a given project.
//
// Returns a list of entities to store to apply the update.
func prepareConsolesUpdate(c context.Context, knownProjects map[string]map[string]*projectconfigpb.Console,
	project *Project, cfg *projectconfigpb.Project, meta *configInterface.Meta) ([]any, error) {
	// If no Milo config was loaded, there are no consoles to update.
	if cfg == nil {
		return []any{}, nil
	}

	// Extract the headers into a map for convenience.
	headers := make(map[string]*projectconfigpb.Header, len(cfg.Headers))
	for _, header := range cfg.Headers {
		headers[header.Id] = header
	}

	// Iterate through all the proto consoles, adding and replacing the
	// known ones if needed.
	toPut := make([]any, 0, len(cfg.Consoles))
	for i, pc := range cfg.Consoles {
		// Resolve the console if it refers to one from a different project.
		if pc.ExternalProject != "" {
			// If ExternalProject is set, validation ensures ExternalId is also set.
			externalConsole, ok := knownProjects[pc.ExternalProject][pc.ExternalId]
			if !ok {
				return nil, fmt.Errorf("external console %s not found at %s:%s", pc.Id, pc.ExternalProject, pc.ExternalId)
			}
			if externalConsole.ExternalId != "" {
				return nil, fmt.Errorf("external console %s found at %s:%s, but was itself an external console",
					pc.Id, pc.ExternalProject, pc.ExternalId)
			}
			// Copy the fields of the external console into this one, but keep Id, Name,
			// ExternalProject, and ExternalId.
			mergedPc := proto.Clone(externalConsole).(*projectconfigpb.Console)
			mergedPc.Id = pc.Id
			mergedPc.Name = pc.Name
			mergedPc.ExternalProject = pc.ExternalProject
			mergedPc.ExternalId = pc.ExternalId
			pc = mergedPc
		}
		if header, ok := headers[pc.HeaderId]; pc.Header == nil && ok {
			// Inject a header if HeaderId is specified, and it doesn't already have one.
			pc.Header = header
		}
		switch con, err := GetConsole(c, project.ID, pc.Id); {
		case err == ErrConsoleNotFound:
			// continue
		case err != nil:
			return nil, errors.Fmt("checking %s: %w", pc.Id, err)
		case con.ConfigRevision == meta.Revision && con.Ordinal == i:
			logging.Debugf(c, "skipping updates")
			// Check if revisions match; if so just skip it.
			// TODO(jchinlee): remove Ordinal check when Version field is added to Console.
			continue
		}

		toPut = append(toPut, &Console{
			Parent:         datastore.KeyForObj(c, project),
			ID:             pc.Id,
			Ordinal:        i,
			ConfigURL:      configURL(c, meta),
			ConfigRevision: meta.Revision,
			Builders:       pc.AllLegacyBuilderIDs(),
			Def:            proto.Clone(pc).(*projectconfigpb.Console),
			Realm:          pc.Realm,
		})
	}
	return toPut, nil
}

// parseProjectACL parses project.cfg and extracts project ACL from it.
func parseProjectACL(projectCfg string) (ACL, error) {
	var cfg luciconfigpb.ProjectCfg
	if err := protoutil.UnmarshalTextML(projectCfg, &cfg); err != nil {
		return ACL{}, err
	}

	// Each entry in cfg.Access is either 'group:xxx', 'user:xxx' or just 'xxx'.
	// Prefix 'user:' is default.
	groups := stringset.New(0)
	idents := stringset.New(0)
	for _, access := range cfg.Access {
		if !strings.ContainsRune(access, ':') {
			access = "user:" + access
		}
		if strings.HasPrefix(access, "group:") {
			groups.Add(strings.TrimPrefix(access, "group:"))
		} else {
			idents.Add(access)
		}
	}

	acl := ACL{
		Groups:     groups.ToSortedSlice(),
		Identities: make([]identity.Identity, idents.Len()),
	}
	for idx, ident := range idents.ToSortedSlice() {
		acl.Identities[idx] = identity.Identity(ident)
	}
	return acl, nil
}

// getConsolesFromMiloCfg extracts a list of consoles from a Milo config proto
// and returns them in a map, indexed by name.
//
// If miloCfg is nil or doesn't have any consoles, returns non-nil empty map.
func getConsolesFromMiloCfg(miloCfg *projectconfigpb.Project) map[string]*projectconfigpb.Console {
	consoles := make(map[string]*projectconfigpb.Console, len(miloCfg.GetConsoles()))
	for _, console := range miloCfg.GetConsoles() {
		consoles[console.Id] = console
	}
	return consoles
}

// UpdateProjects reads project configs from LUCI Config and updates entities.
//
// Visits all LUCI projects (not only ones that have Milo config) to grab
// their visibility ACL from project.cfg file.
func UpdateProjects(c context.Context) error {
	lucicfg := cfgclient.Client(c)

	// Fetch project.cfg with ACLs. Every LUCI project has it, so we effectively
	// enumerate all LUCI projects.
	logging.Debugf(c, "fetching all project.cfg...")
	cfgs, err := lucicfg.GetProjectConfigs(c, "project.cfg", false)
	if err != nil {
		return errors.Fmt("while fetching project configs: %w", err)
	}
	logging.Debugf(c, "found %d LUCI projects", len(cfgs))

	// This will collect results of individual fetchProject call. It will receive
	// exactly len(cfgs) items no matter what.
	type result struct {
		projectID   string
		project     *Project
		miloCfg     *projectconfigpb.Project
		miloCfgMeta *configInterface.Meta
		err         error
	}
	results := make(chan result, len(cfgs))

	// Fetch project configs in parallel to make sure we fit under 10 min cron job
	// deadline. Each task is simple, but involves a slow RPC to LUCI Config.
	// We don't want to run all of them sequentially.
	parallel.WorkPool(8, func(tasks chan<- func() error) {
		for _, cfg := range cfgs {
			tasks <- func() error {
				project, miloCfg, miloCfgMeta, err := fetchProject(c, &cfg)
				results <- result{
					projectID:   cfg.ConfigSet.Project(),
					project:     project,
					miloCfg:     miloCfg,
					miloCfgMeta: miloCfgMeta,
					err:         errors.WrapIf(err, "fetching %q", cfg.ConfigSet),
				}
				return nil
			}
		}
	})

	// Build a map "project name -> set of consoles in it", and gather all errors.
	// We use the map for two purposes:
	//   * Resolving external console references
	//   * Pruning consoles that were deleted.
	// Projects with broken configs that could not be fetched are represented by
	// nil entries. Projects without consoles are represented by empty maps.
	knownProjects := map[string]map[string]*projectconfigpb.Console{}
	merr := errors.MultiError{}
	resultsList := make([]result, 0, len(cfgs))
	for range cfgs {
		res := <-results
		if res.err == nil {
			knownProjects[res.projectID] = getConsolesFromMiloCfg(res.miloCfg)
		} else {
			knownProjects[res.projectID] = nil
			merr = append(merr, res.err)
		}
		resultsList = append(resultsList, res)
	}

	// Now process the consoles for each project and push them to Datastore, resolving
	// external console references along the way.
	err = parallel.FanOutIn(func(tasks chan<- func() error) {
		for _, res := range resultsList {
			// If there was an error fetching this project, don't apply any Datastore changes.
			if res.err != nil {
				continue
			}
			tasks <- func() error {
				// Apply datastore changes in a single transaction per project.
				err := datastore.RunInTransaction(c, func(c context.Context) error {
					toPut, err := prepareConsolesUpdate(c, knownProjects, res.project, res.miloCfg, res.miloCfgMeta)
					if err != nil {
						return err
					}
					toPut = append(toPut, res.project)
					return datastore.Put(c, toPut)
				}, nil)
				if res.miloCfgMeta != nil {
					return errors.WrapIf(err, "when applying config rev %q of %q", res.miloCfgMeta.Revision, res.miloCfgMeta.ConfigSet)
				} else {
					return errors.WrapIf(err, "when updating config for %q", res.project.ID)
				}
			}
		}
	})
	if err != nil {
		merr = append(merr, err.(errors.MultiError)...)
	}

	toDelete := []*datastore.Key{}

	// Find all the consoles that no longer exist or are part of deleted projects.
	logging.Debugf(c, "searching for stale consoles")
	err = datastore.Run(c, datastore.NewQuery("Console"), func(key *datastore.Key) error {
		proj := key.Parent().StringID()
		id := key.StringID()
		// If this console is either:
		// 1. In a project that no longer exists, or
		// 2. Not in the project, then delete it.
		knownConsoles, ok := knownProjects[proj]
		if !ok {
			logging.Infof(
				c, "deleting %s/%s because the project no longer exists", proj, id)
			toDelete = append(toDelete, key)
			return nil
		}
		if knownConsoles == nil {
			// The project exists but we couldn't check it this time.  Skip it and
			// try again the next cron cycle.
			return nil
		}
		if _, ok := knownConsoles[id]; !ok {
			logging.Infof(
				c, "deleting %s/%s because the console no longer exists", proj, id)
			toDelete = append(toDelete, key)
		}
		return nil
	})
	if err != nil {
		merr = append(merr, err)
	}

	// Find entities of no longer existing projects.
	logging.Debugf(c, "searching for stale projects")
	err = datastore.Run(c, datastore.NewQuery("Project"), func(key *datastore.Key) error {
		proj := key.StringID()
		if _, ok := knownProjects[proj]; !ok {
			logging.Infof(c, "deleting Project entity for %s", proj)
			toDelete = append(toDelete, key)
		}
		return nil
	})
	if err != nil {
		merr = append(merr, err)
	}

	// Actually delete all stale entities.
	if len(toDelete) != 0 {
		logging.Debugf(c, "deleting %d stale entities", len(toDelete))
		if err := datastore.Delete(c, toDelete); err != nil {
			merr = append(merr, err)
		}
	}

	// Print some stats.
	processedConsoles := 0
	for _, cons := range knownProjects {
		processedConsoles += len(cons)
	}
	logging.Infof(c, "processed %d consoles over %d projects", processedConsoles, len(knownProjects))

	if len(merr) == 0 {
		return nil
	}
	return merr
}

func consolesCacheKey(builderID string) string {
	return "milo:consolesCacheKey:" + builderID
}

// GetAllConsoles returns all Consoles (across all projects) which contain the
// builder ID. If builderID is empty, then this retrieves all Consoles.
//
// TODO-perf(iannucci): Maybe memcache this too.
func GetAllConsoles(c context.Context, builderID string) ([]*Console, error) {
	itm, err := caching.RequestCache(c).GetOrCreate(c, consolesCacheKey(builderID), func() (any, time.Duration, error) {
		q := datastore.NewQuery("Console")
		if builderID != "" {
			q = q.Eq("Builders", builderID)
		}
		con := []*Console{}
		err := datastore.GetAll(c, q, &con)

		return con, 0, transient.Tag.Apply(
			errors.WrapIf(err, "getting consoles for %q", builderID))
	})
	con, _ := itm.([]*Console)
	return con, err
}

// GetProject loads the project from the datastore.
func GetProject(c context.Context, project string) (*Project, error) {
	proj := Project{
		ID: project,
	}
	err := datastore.Get(c, &proj)
	if err == datastore.ErrNoSuchEntity {
		err = nil
	}
	return &proj, errors.WrapIf(err, "getting project %q", project)
}

// GetVisibleProjects returns all projects with consoles the current user has
// access to.
//
// Skips projects that do not have Milo config file.
func GetVisibleProjects(c context.Context) ([]*Project, error) {
	q := datastore.NewQuery("Project").Eq("HasConfig", true)
	projs := []*Project{}

	if err := datastore.GetAll(c, q, &projs); err != nil {
		return nil, errors.Fmt("getting projects: %w", err)
	}
	result := []*Project{}
	for _, proj := range projs {
		switch allowed, err := CheckACL(c, proj.ID, proj.ACL); {
		case err != nil:
			return nil, err
		case allowed:
			result = append(result, proj)
		}
	}
	return result, nil
}

func getVisibleProjectIDs(c context.Context) (stringset.Set, error) {
	visibleProjects, err := GetVisibleProjects(c)
	if err != nil {
		return nil, err
	}
	projectIDs := stringset.New(len(visibleProjects))
	for _, p := range visibleProjects {
		projectIDs.Add(p.ID)
	}
	return projectIDs, nil
}

// filterExternalConsoles takes a slice of Consoles and filters out any external
// consoles that the user does not have access to. (It is assumed that the user
// can see all non-external consoles by virtue of having access to the Project.)
func filterExternalConsoles(c context.Context, consoles []*Console, visibleProjects stringset.Set) []*Console {
	res := make([]*Console, 0, len(consoles))
	for _, c := range consoles {
		if c.IsExternal() && !visibleProjects.Has(c.Def.ExternalProject) {
			continue
		}
		res = append(res, c)
	}
	return res
}

// GetProjectConsoles returns all consoles for the given project ordered as in config.
// Excludes references to external consoles that the user does not have access to.
func GetProjectConsoles(c context.Context, projectID string) ([]*Console, error) {
	// Query datastore for consoles related to the project.
	q := datastore.NewQuery("Console")
	parentKey := datastore.MakeKey(c, "Project", projectID)
	q = q.Ancestor(parentKey)
	con := []*Console{}
	if err := datastore.GetAll(c, q, &con); err != nil {
		return nil, errors.Fmt("getting project %q consoles: %w", projectID, err)
	}
	sort.Slice(con, func(i, j int) bool { return con[i].Ordinal < con[j].Ordinal })
	visibleProjects, err := getVisibleProjectIDs(c)
	if err != nil {
		return nil, errors.Fmt("getting visible projects: %w", err)
	}
	return filterExternalConsoles(c, con, visibleProjects), nil
}

// GetConsole returns the requested console.
//
// TODO-perf(iannucci,hinoka): Memcache this.
func GetConsole(c context.Context, proj, id string) (*Console, error) {
	con := Console{
		Parent: datastore.MakeKey(c, "Project", proj),
		ID:     id,
	}
	switch err := datastore.Get(c, &con); err {
	case datastore.ErrNoSuchEntity:
		return nil, ErrConsoleNotFound
	case nil:
		return &con, nil
	default:
		return nil, errors.Fmt("getting project %q console %q: %w", proj, id, err)
	}
}

// GetConsoles returns the requested consoles.
// Excludes references to external consoles that the user does not have access to.
//
// TODO-perf(iannucci,hinoka): Memcache this.
func GetConsoles(c context.Context, consoles []ConsoleID) ([]*Console, error) {
	result := make([]*Console, len(consoles))
	for i, con := range consoles {
		result[i] = con.SetID(c, nil)
	}
	if err := datastore.Get(c, result); err != nil {
		err = utils.ReplaceNSEWith(err.(errors.MultiError), ErrConsoleNotFound)
		return result, errors.Fmt("getting %s consoles: %w", consoles, err)
	}
	visibleProjects, err := getVisibleProjectIDs(c)
	if err != nil {
		return nil, errors.Fmt("getting visible projects: %w", err)
	}
	return filterExternalConsoles(c, result, visibleProjects), nil
}

// UpdateProjectConfigsHandler is an HTTP handler that handles project
// configuration update requests.
func UpdateProjectConfigsHandler(c context.Context) error {
	err := UpdateProjects(c)
	if err != nil {
		if merr, ok := err.(errors.MultiError); ok {
			for _, ierr := range merr {
				logging.WithError(ierr).Errorf(c, "project update handler encountered error")
			}
		} else {
			logging.WithError(err).Errorf(c, "project update handler encountered error")
		}
	}

	return err
}
