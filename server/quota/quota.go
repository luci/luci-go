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
	"go.chromium.org/luci/server/redisconn"

	pb "go.chromium.org/luci/server/quota/proto"
	"go.chromium.org/luci/server/quota/quotaconfig"
)

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
		panic(errors.Reason("quotaconfig.Interface implementation not found (ensure quota.Use is called in server.Main)").Err())
	}
	return cfg
}

type Options struct {
	// User is a value to substitute for ${user} in policy names.
	// Policy defaults are sourced from the unsubstituted name.
	User string
}

// debitEntry is a *template.Template for a Lua script which fetches the given
// quota entry from Redis, initializing it if it doesn't exist, replenishes
// quota since the last update, and debits the specified amount. Does not modify
// the entry in the database, instead entries must explicitly be stored by
// concatenating this script with setEntry. This enables atomic debiting of
// multiple entries by running a script which concatenates multiple debitEntry
// calls followed by corresponding setEntry calls. Such a script updates entries
// in the database iff all debits would succeed.
//
// Template variables:
// Var: Name of a Lua variable to store the quota entry in memory.
// Name: Name of the quota entry to debit.
// Default: Default number of resources to initialize new entries with.
// Now: Update time in seconds since epoch.
// Replenishment: Amount of resources to replenish every second.
// Amount: Amount of resources to debit.
var debitEntry = template.Must(template.New("debitEntry").Parse(`
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

	-- Replenish resources before debiting.
	{{.Var}}["resources"] = {{.Var}}["resources"] + ({{.Now}} - {{.Var}}["updated"]) * {{.Replenishment}}
	{{.Var}}["updated"] = {{.Now}}
	if {{.Var}}["resources"] - {{.Amount}} < 0 then
		return redis.error_reply("\"{{.Name}}\" has insufficient resources")
	end
	{{.Var}}["resources"] = {{.Var}}["resources"] - {{.Amount}}
	{{.Var}}["updated"] = {{.Now}}

	-- Cap resources at the default amount.
	if {{.Var}}["resources"] > {{.Default}} then
		{{.Var}}["resources"] = {{.Default}}
	end
`))

// setEntry is a *template.Template for a Lua script which sets the given quota
// entry in Redis. Should be used after debitEntry.
//
// Template variables:
// Var: Name of a Lua variable holding the quota entry in memory.
var setEntry = template.Must(template.New("setEntry").Parse(`
	redis.call("HMSET", {{.Var}}["name"], "resources", {{.Var}}["resources"], "updated", {{.Var}}["updated"])
`))

// DebitQuota debits the given quota entries atomically.
//
// Panics if quotaconfig.Interface is not available in the given context.Context
// (see WithConfig).
func DebitQuota(ctx context.Context, debits map[string]int64, opts *Options) error {
	now := clock.Now(ctx).Unix()
	cfg := getInterface(ctx)

	defs := make(map[string]*pb.Policy, len(debits))
	adjs := make(map[string]int64, len(debits))

	i := 0
	for pol, val := range debits {
		name := pol
		if strings.Contains(pol, "${user}") {
			if opts == nil || opts.User == "" {
				return errors.Reason("user unspecified for %q", pol).Err()
			}
			name = strings.ReplaceAll(name, "${user}", opts.User)
		}
		name = fmt.Sprintf("entry:%x", sha256.Sum256([]byte(name)))
		def, err := cfg.Get(ctx, pol)
		if err != nil {
			return errors.Annotate(err, "fetching config").Err()
		}
		defs[name] = def
		adjs[name] = val
		i++
	}

	conn, err := redisconn.Get(ctx)
	if err != nil {
		return errors.Annotate(err, "establishing connection").Err()
	}
	defer conn.Close()

	s := bytes.NewBufferString("local entries = {}\n")
	i = 0
	for name, adj := range adjs {
		if err := debitEntry.Execute(s, map[string]interface{}{
			"Var":           fmt.Sprintf("entries[%d]", i),
			"Name":          name,
			"Default":       defs[name].Resources,
			"Now":           now,
			"Replenishment": defs[name].Replenishment,
			"Amount":        adj,
		}); err != nil {
			return errors.Annotate(err, "rendering template %q", debitEntry.Name()).Err()
		}
		i++
	}
	for i--; i >= 0; i-- {
		if err := setEntry.Execute(s, map[string]interface{}{
			"Var": fmt.Sprintf("entries[%d]", i),
		}); err != nil {
			return errors.Annotate(err, "rendering template %q", setEntry.Name()).Err()
		}
	}

	if _, err := redis.NewScript(0, s.String()).Do(conn); err != nil {
		return err
	}
	return nil
}
