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

package quota

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"text/template"

	"github.com/gomodule/redigo/redis"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/server/quotabeta/proto"
	"go.chromium.org/luci/server/quotabeta/quotaconfig"
	"go.chromium.org/luci/server/redisconn"
)

// ErrInsufficientQuota is returned by UpdateQuota when the updates were not
// applied due to insufficient quota.
var ErrInsufficientQuota = errors.New("insufficient quota")

var cfgKey = "cfg"

// Use returns a context.Context directing this package to use the given
// quotaconfig.Interface.
func Use(ctx context.Context, cfg quotaconfig.Interface) context.Context {
	return context.WithValue(ctx, &cfgKey, cfg)
}

// getInterface returns the quotaconfig.Interface available in the given
// context.Context. Panics if no quotaconfig.Interface is available (see Use).
func getInterface(ctx context.Context) quotaconfig.Interface {
	cfg, ok := ctx.Value(&cfgKey).(quotaconfig.Interface)
	if !ok {
		panic(errors.New("quotaconfig.Interface implementation not found (ensure quota.Use is called in server.Main)"))
	}
	return cfg
}

type Options struct {
	// RequestID is a string to use for deduplication of successful quota
	// adjustments, valid for one hour. Only successful updates are deduplicated,
	// mainly for the case where time has passed and quota has replenished
	// enough for a previously failed adjustment to succeed.
	//
	// Callers should ensure the same Options and function parameters are provided
	// each time a request ID is reused, since any reuse within a one hour period
	// is subject to deduplication regardless of other Options and parameters.
	RequestID string
	// User is a value to substitute for ${user} in policy names.
	// Policy defaults are sourced from the unsubstituted name.
	User string
}

// dedupePrefix is a *template.Template for a Lua script which fetches the given
// deduplication key from Redis, returning if it exists and is temporally valid.
// Prefix to other scripts to create a script which exits early when the
// deduplication key is valid. Scripts should be suffixed with dedupeSuffix to
// save the deduplication key.
//
// Template variables:
// Key: A string to use for deduplication.
// Now: Current time in seconds since epoch.
var dedupePrefix = template.Must(template.New("dedupePrefix").Parse(`
	local deadline = redis.call("HINCRBY", "deduplicationKeys", "{{.Key}}", 0)
	if deadline >= {{.Now}} then
		return
	end
`))

// dedupeSuffix is a *template.Template for a Lua script which writes the given
// deduplication key to Redis. Should be used with dedupePrefix.
//
// Template variables:
// Key: A string to use for deduplication.
// Deadline: Time in seconds since epoch after which the key no longer dedupes.
var dedupeSuffix = template.Must(template.New("dedupeSuffix").Parse(`
	redis.call("HMSET", "deduplicationKeys", "{{.Key}}", {{.Deadline}})
`))

// updateEntry is a *template.Template for a Lua script which fetches the given
// quota entry from Redis, initializing it if it doesn't exist, replenishes
// quota since the last update, and updates the amount. Does not modify the
// entry in the database, instead entries must explicitly be stored by
// concatenating this script with setEntry. This enables atomic updates of
// multiple entries by running a script which concatenates multiple updateEntry
// calls followed by corresponding setEntry calls. Such a script updates entries
// in the database iff all updates would succeed.
//
// Template variables:
// Var: Name of a Lua variable to store the quota entry in memory.
// Name: Name of the quota entry to update.
// Default: Default number of resources to initialize new entries with.
// Now: Update time in seconds since epoch.
// Replenishment: Amount of resources to replenish every second.
// Amount: Amount by which to update resources.
var updateEntry = template.Must(template.New("updateEntry").Parse(`
	{{.Var}} = {}
	{{.Var}}["name"] = "{{.Name}}"

	-- Check last updated time to determine if this entry exists.
	{{.Var}}["updated"] = redis.call("HINCRBY", "{{.Name}}", "updated", 0)
	if {{.Var}}["updated"] == 0 then
		-- Delete the updated time of 0 to avoid leaving partial entries
		-- in the database in case of error.
		redis.call("DEL", "{{.Name}}")
		{{.Var}}["resources"] = {{.Default}}
		{{.Var}}["updated"] = {{.Now}}
	elseif {{.Var}}["updated"] > {{.Now}} then
		return redis.error_reply("\"{{.Name}}\" last updated in the future")
	else
		{{.Var}}["resources"] = redis.call("HINCRBY", "{{.Name}}", "resources", 0)
	end

	-- Replenish resources up to the cap before updating.
	{{.Var}}["resources"] = {{.Var}}["resources"] + ({{.Now}} - {{.Var}}["updated"]) * {{.Replenishment}}
	{{.Var}}["updated"] = {{.Now}}
	-- Cap resources at the default amount.
	if {{.Var}}["resources"] > {{.Default}} then
		{{.Var}}["resources"] = {{.Default}}
	end

	-- Check that the update would succeed before updating.
	if {{.Var}}["resources"] + {{.Amount}} < 0 then
		return redis.error_reply("\"{{.Name}}\" has insufficient resources")
	end

	-- Update resources up to the cap.
	{{.Var}}["resources"] = {{.Var}}["resources"] + {{.Amount}}
	{{.Var}}["updated"] = {{.Now}}
	-- Cap resources at the default amount.
	if {{.Var}}["resources"] > {{.Default}} then
		{{.Var}}["resources"] = {{.Default}}
	end
`))

// setEntry is a *template.Template for a Lua script which sets the given quota
// entry in Redis. Should be used after updateEntry.
//
// Template variables:
// Var: Name of a Lua variable holding the quota entry in memory.
var setEntry = template.Must(template.New("setEntry").Parse(`
	redis.call("HMSET", {{.Var}}["name"], "resources", {{.Var}}["resources"], "updated", {{.Var}}["updated"])
`))

// UpdateQuota atomically adjusts the given quota entries using the given map of
// policy names to numeric update amounts as well as the given *Options. Returns
// ErrInsufficientQuota when the adjustments were not made due to insufficient
// quota.
//
// Panics if quotaconfig.Interface is not available in the given context.Context
// (see WithConfig).
func UpdateQuota(ctx context.Context, updates map[string]int64, opts *Options) error {
	now := clock.Now(ctx).Unix()
	cfg := getInterface(ctx)

	defs := make(map[string]*pb.Policy, len(updates))
	adjs := make(map[string]int64, len(updates))

	i := 0
	for pol, val := range updates {
		name := pol
		if strings.Contains(pol, "${user}") {
			if opts == nil || opts.User == "" {
				return errors.Fmt("user unspecified for %q", pol)
			}
			name = strings.ReplaceAll(name, "${user}", opts.User)
		}
		name = fmt.Sprintf("entry:%x", sha256.Sum256([]byte(name)))
		def, err := cfg.Get(ctx, pol)
		if err != nil {
			return errors.Fmt("fetching config: %w", err)
		}
		defs[name] = def
		adjs[name] = val
		i++
	}

	conn, err := redisconn.Get(ctx)
	if err != nil {
		return errors.Fmt("establishing connection: %w", err)
	}
	defer conn.Close()

	s := bytes.NewBufferString("local entries = {}\n")
	if opts != nil && opts.RequestID != "" {
		if err := dedupePrefix.Execute(s, map[string]any{
			"Key": opts.RequestID,
			"Now": now,
		}); err != nil {
			return errors.Fmt("rendering template %q: %w", dedupePrefix.Name(), err)
		}
	}

	i = 0
	for name, adj := range adjs {
		if err := updateEntry.Execute(s, map[string]any{
			"Var":           fmt.Sprintf("entries[%d]", i),
			"Name":          name,
			"Default":       defs[name].Resources,
			"Now":           now,
			"Replenishment": defs[name].Replenishment,
			"Amount":        adj,
		}); err != nil {
			return errors.Fmt("rendering template %q: %w", updateEntry.Name(), err)
		}
		i++
	}
	for i--; i >= 0; i-- {
		if err := setEntry.Execute(s, map[string]any{
			"Var": fmt.Sprintf("entries[%d]", i),
		}); err != nil {
			return errors.Fmt("rendering template %q: %w", setEntry.Name(), err)
		}
	}

	if opts != nil && opts.RequestID != "" {
		if err := dedupeSuffix.Execute(s, map[string]any{
			"Key":      opts.RequestID,
			"Deadline": now + 3600,
		}); err != nil {
			return errors.Fmt("rendering template %q: %w", dedupeSuffix.Name(), err)
		}
	}

	if _, err := redis.NewScript(0, s.String()).Do(conn); err != nil {
		if strings.HasSuffix(err.Error(), "insufficient resources") {
			return ErrInsufficientQuota
		}
		return err
	}
	return nil
}
