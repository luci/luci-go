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

package cfg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"regexp"
	"sort"
	"time"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/service/datastore"

	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/cfg/internalcfgpb"
	"go.chromium.org/luci/swarming/server/cipd"
)

// Individually recognized config files.
const (
	settingsCfg = "settings.cfg"
	poolsCfg    = "pools.cfg"
	botsCfg     = "bots.cfg"

	// The main hooks file embedded into the bot archive.
	botConfigPy = "scripts/bot_config.py"
)

// hookScriptRe matches paths like `scripts/hooks.py`. It intentionally doesn't
// match subdirectories of `scripts/` since they contain unit tests the server
// doesn't care about.
var hookScriptRe = regexp.MustCompile(`scripts/[^/]+\.py`)

const (
	// A pseudo-revision of an empty config.
	emptyRev = "0000000000000000000000000000000000000000"
	// A digest of a default *.cfg configs (calculated in the test).
	emptyCfgDigest = "0NpkIis/WMci8PDKkLD3PB/t8B86nbBVjyD59iosjOM"
)

// BotChannel is either StableBot or CanaryBot.
type BotChannel bool

const (
	StableBot BotChannel = false
	CanaryBot BotChannel = true
)

// String returns either "stable" or "canary".
func (c BotChannel) String() string {
	switch c {
	case StableBot:
		return "stable"
	case CanaryBot:
		return "canary"
	default:
		panic("impossible")
	}
}

// CIPD is used to communicate with the CIPD server.
//
// Usually it is *cipd.Client.
type CIPD interface {
	// ResolveVersion resolves a version label into a CIPD instance ID.
	ResolveVersion(ctx context.Context, server, cipdpkg, version string) (string, error)
	// FetchInstance fetches contents of a package given via its instance ID.
	FetchInstance(ctx context.Context, server, cipdpkg, iid string) (pkg.Instance, error)
}

// EmbeddedBotSettings is configuration data that eventually ends up in the
// bot zip archive inside config.json.
type EmbeddedBotSettings struct {
	// ServerURL is "https://..." URL of the Swarming server itself.
	ServerURL string
}

// digest is a digest of the struct content.
func (ems *EmbeddedBotSettings) digest() string {
	return digest(map[string]string{
		"ServerURL": ems.ServerURL,
	})
}

// VersionInfo identifies versions of Swarming configs.
//
// It is stored in the datastore as part of ConfigBundle and ConfigBundleRev.
type VersionInfo struct {
	// Digest is deterministically derived from the fully expanded configs
	// (including the bot code archive fetched from CIPD).
	//
	// It is used by the config cache to know if something has really changed
	// and the in-memory copy of the config needs to be reloaded.
	Digest string `gae:",noindex"`

	// Fetched is when this config content was fetched for the first time.
	//
	// This is the first time this specific config digest was seen. Always
	// monotonically increases. Has milliseconds precision.
	Fetched time.Time `gae:",noindex"`

	// Revision is the config repo git commit processed most recently.
	//
	// Note that Revision may changed even if Digest stays the same (happens if
	// some config file we don't use changes).
	//
	// This is FYI mostly. Digest is used in all important places to figure out
	// if config really changes.
	Revision string `gae:",noindex"`

	// Touched is when this config was last touched by the UpdateConfig cron.
	//
	// This is FYI mostly. It is updated whenever a new revision is seen, even if
	// it doesn't change the digest. Has milliseconds precision.
	Touched time.Time `gae:",noindex"`

	// StableBot is information about the current stable bot archive version.
	//
	// Derived based on the BotDeployment config section and the state in CIPD.
	StableBot BotArchiveInfo `gae:",noindex"`

	// CanaryBot is information about the current canary bot archive version.
	//
	// Derived based on the BotDeployment config section and the state in CIPD.
	CanaryBot BotArchiveInfo `gae:",noindex"`
}

// BotArchiveInfo contains information about a concrete assembled bot archive.
//
// It contains both some inputs to the bot archive building process and its
// output: the bot archive digest and references to its chunks in the datastore
// that can be assembled together to get the final zip file.
//
// Inputs are available through the settings.cfg config as well, but they are
// copied here for convenience when logging changes.
type BotArchiveInfo struct {
	// Digest is the bot archive SHA256 digest aka "bot archive version".
	Digest string `gae:",noindex"`
	// Chunks is the list of BotArchiveChunk entities with the archive content.
	Chunks []string `gae:",noindex"`

	// BotConfigHash is SHA256 of the bot_config.py embedded into the bot archive.
	BotConfigHash string `gae:",noindex"`
	// BotConfigRev is the revision of bot_config.py script used.
	BotConfigRev string `gae:",noindex"`

	// PackageInstanceID is the CIPD instance ID of the base bot package.
	PackageInstanceID string `gae:",noindex"`
	// PackageServer is an URL of the CIPD server with the bot package.
	PackageServer string `gae:",noindex"`
	// PackageName is the CIPD package name e.g. "luci/swarming/swarming_bot".
	PackageName string `gae:",noindex"`
	// PackageVersion is e.g. "git_commit:..." CIPD tag or ref.
	PackageVersion string `gae:",noindex"`
}

// logDiff logs changes in the struct (if any).
func (ba *BotArchiveInfo) logDiff(ctx context.Context, channel string, new BotArchiveInfo) {
	if ba.Digest != new.Digest {
		logging.Infof(ctx, "%s bot digest: %s => %s", channel, ba.Digest, new.Digest)
	}
	if ba.PackageVersion != new.PackageVersion {
		logging.Infof(ctx, "%s version: %s => %s", channel, ba.PackageVersion, new.PackageVersion)
	}
	if ba.PackageInstanceID != new.PackageInstanceID {
		logging.Infof(ctx, "%s instance: %s => %s", channel, ba.PackageInstanceID, new.PackageInstanceID)
	}
	// Log revisions only when the content actually changes. Don't log the hash
	// itself, no one really cares.
	if ba.BotConfigHash != new.BotConfigHash {
		logging.Infof(ctx, "%s bot_config.py: %s => %s", channel, ba.BotConfigRev, new.BotConfigRev)
	}
}

// FetchBotArchive fetches the bot archive blob from the datastore.
func (ba *BotArchiveInfo) FetchBotArchive(ctx context.Context) ([]byte, error) {
	return fetchBotArchive(ctx, ba.Chunks)
}

// Config is an immutable queryable representation of Swarming server configs.
//
// It is a snapshot of configs at some particular revision. Use an instance of
// Provider to get it.
type Config struct {
	// VersionInfo contains versions of the fetched configs.
	VersionInfo VersionInfo
	// Refreshed is the local time when the config was fetched from the datastore.
	Refreshed time.Time
	// DefaultCIPD is the default CIPD to use, including CIPD server and the
	// client package.
	DefaultCIPD *configpb.ExternalServices_CIPD

	settings  *configpb.SettingsCfg
	traffic   map[string]*configpb.TrafficMigration_Route
	poolMap   map[string]*Pool // pool name => config
	poolNames []string         // sorted list of pool names
	botGroups *botGroups       // can map bot ID to a bot group config
}

// Settings are settings proto with defaults filled in.
func (cfg *Config) Settings() *configpb.SettingsCfg {
	return cfg.settings
}

// Pool returns a config for the given pool or nil if there's no such pool.
func (cfg *Config) Pool(name string) *Pool {
	return cfg.poolMap[name]
}

// Pools returns a sorted list of all known pools.
func (cfg *Config) Pools() []string {
	return cfg.poolNames
}

// BotChannel returns what release channel and archive a bot should be using.
func (cfg *Config) BotChannel(botID string) (BotChannel, *BotArchiveInfo) {
	// Get a quasi random integer in range [0; 100) by hashing the bot ID.
	// Do exactly what the Python code is doing to avoid bots flapping between
	// modes when migrating Python => Go. This also uses a different hash seed to
	// avoid "synchronizing" with a similar hash calculation in pools.go.
	sum := sha256.Sum256(bytes.Join([][]byte{
		[]byte("bot-channel:"),
		[]byte(botID),
	}, nil))
	num := float32(sum[0]) + float32(sum[1])*256.0
	rnd := int32(num * 99.9 / (256.0 + 256.0*256.0))
	if rnd < cfg.settings.GetBotDeployment().GetCanaryPercent() {
		return CanaryBot, &cfg.VersionInfo.CanaryBot
	}
	return StableBot, &cfg.VersionInfo.StableBot
}

// BotGroup returns a BotGroup config matching the given bot ID.
//
// Understands composite bot IDs, see HostBotID(...). Always returns some
// config (never nil). If there's no config assigned to a bot, returns a default
// config.
func (cfg *Config) BotGroup(botID string) *BotGroup {
	hostID := HostBotID(botID)

	// If this is a composite bot ID, try to find if there's a config for this
	// *specific* composite ID first. This acts as an override if we need to
	// single-out a bot that uses a concrete composite IDs.
	if hostID != botID {
		if group := cfg.botGroups.directMatches[botID]; group != nil {
			return group
		}
	}

	// Otherwise look it up based on the host ID (which is the same as bot ID
	// for non-composite IDs).
	if group := cfg.botGroups.directMatches[hostID]; group != nil {
		return group
	}
	if _, group, ok := cfg.botGroups.prefixMatches.LongestPrefix(hostID); ok {
		return group.(*BotGroup)
	}
	return cfg.botGroups.defaultGroup
}

// RBEConfig returns RBE-related configuration that applies to the given bot.
//
// It checks per-pool RBE configs for all pools the bot belongs to and "merges"
// them into the final config.
func (cfg *Config) RBEConfig(botID string) RBEConfig {
	// For each known pool (usually just one) calculate the mode and the RBE
	// instance the bot should be using there.
	pools := cfg.BotGroup(botID).Pools()
	perPool := make([]RBEConfig, 0, len(pools))
	for _, pool := range pools {
		if poolCfg := cfg.Pool(pool); poolCfg != nil {
			perPool = append(perPool, poolCfg.rbeConfig(botID))
		}
	}

	// If the bot is only in unknown pools, assume it is a Swarming bot (since we
	// don't know an RBE instance name to use for it). Such bot will not be able
	// to poll for tasks anyway.
	if len(perPool) == 0 {
		return RBEConfig{
			Mode: configpb.Pool_RBEMigration_BotModeAllocation_SWARMING,
		}
	}

	// If all pools agree on a single mode, use it whatever it is. Otherwise use
	// HYBRID, since it is compatible with all modes.
	var mode configpb.Pool_RBEMigration_BotModeAllocation_BotMode
	for i, poolCfg := range perPool {
		if i == 0 {
			mode = poolCfg.Mode
		} else if poolCfg.Mode != mode {
			mode = configpb.Pool_RBEMigration_BotModeAllocation_HYBRID
			break
		}
	}

	// Pure Swarming bots have no RBE instance.
	if mode == configpb.Pool_RBEMigration_BotModeAllocation_SWARMING {
		return RBEConfig{
			Mode: configpb.Pool_RBEMigration_BotModeAllocation_SWARMING,
		}
	}

	// RBE pools must be configured to agree on what RBE instance to use. Pick
	// the first set instance.
	instance := ""
	for _, poolCfg := range perPool {
		if poolCfg.Instance != "" {
			instance = poolCfg.Instance
			break
		}
	}
	if instance == "" {
		panic("no RBE instance set in RBEMigration")
	}
	return RBEConfig{
		Mode:     mode,
		Instance: instance,
	}
}

// RouteToGoPercent returns how much traffic to this route should be handled by
// the Go server (vs Python server).
//
// Returns a number in range [0; 100].
func (cfg *Config) RouteToGoPercent(route string) int {
	if r, ok := cfg.traffic[route]; ok {
		return int(r.RouteToGoPercent)
	}
	// Route to Python by default.
	return 0
}

// UpdateConfigs fetches the most recent server configs and bot code and stores
// them in the local datastore if they appear to be valid.
//
// If `cipdClient` is not nil, uses it to communicate with CIPD. Otherwise
// constructs a default client.
//
// Called from a cron job once a minute.
func UpdateConfigs(ctx context.Context, ebs *EmbeddedBotSettings, cipdClient CIPD) error {
	// Fetch known config files and everything that looks like a hooks script.
	files, err := cfgclient.Client(ctx).GetConfigs(ctx, "services/${appid}",
		func(path string) bool {
			return path == settingsCfg || path == poolsCfg || path == botsCfg || hookScriptRe.MatchString(path)
		}, false)
	if err != nil && !errors.Is(err, config.ErrNoConfig) {
		return errors.Annotate(err, "failed to fetch the most recent configs from LUCI Config").Err()
	}

	// Version information about the config we are fetching now. Will be populated
	// field by field when data is fetched.
	var fresh VersionInfo
	if len(files) == 0 {
		// This can happen in new deployments.
		logging.Warningf(ctx, "There are no configuration files in LUCI Config")
		fresh.Revision = emptyRev
	} else {
		// Per GetConfigs logic, all files are at the same revision. Pick the first.
		for _, cfg := range files {
			fresh.Revision = cfg.Revision
			break
		}
	}

	// Parse and re-validate the fetched config to get bot package versions.
	bundle, err := parseAndValidateConfigs(ctx, fresh.Revision, files)
	if err != nil {
		return errors.Annotate(err, "bad configs at rev %s", fresh.Revision).Err()
	}

	// Build the bot archive packages if they are missing. This process is
	// deterministic and idempotent with results stored in the datastore. Inputs
	// are the CIPD package, bot_config.py and EmbeddedBotSettings. Outputs are
	// a bot zip archive stored in the datastore (in chunks) and its digest. If
	// such inputs have already been processed, it is mostly noop.
	//
	// TODO(vadimsh): Start requiring bot_deployment section once this is ready
	// for the final rollout. For now this section is optional and present only
	// on the dev instance.
	if stable := bundle.Settings.GetBotDeployment().GetStable(); stable != nil {
		canary := bundle.Settings.GetBotDeployment().GetCanary()
		if canary == nil {
			canary = stable
		}

		// The hooks file to embed into the bot archive.
		var botConfigBody []byte
		if file, ok := files[botConfigPy]; ok {
			botConfigBody = []byte(file.Content)
		}

		// Build archives sequentially since often the canary and the stable resolve
		// to the same thing and the second call will just quickly discover
		// everything is already done. Also we are in no rush in this background
		// cron job.
		if cipdClient == nil {
			cipdClient = &cipd.Client{}
		}
		fresh.StableBot, err = ensureBotArchiveBuilt(ctx, cipdClient, stable,
			botConfigBody, fresh.Revision, ebs, botArchiveChunkSize)
		if err != nil {
			return errors.Annotate(err, "failed build the stable bot archive").Err()
		}
		fresh.CanaryBot, err = ensureBotArchiveBuilt(ctx, cipdClient, canary,
			botConfigBody, fresh.Revision, ebs, botArchiveChunkSize)
		if err != nil {
			return errors.Annotate(err, "failed build the canary bot archive").Err()
		}
	}

	// The config is fully expanded at this point and we can calculate its digest
	// to figure out if anything has really changed. See also emptyVersionInfo().
	fresh.Digest = digest(map[string]string{
		"configs": bundle.Digest,          // various *.cfg configs and things they reference
		"stable":  fresh.StableBot.Digest, // the stable bot package content + bot_config.py + EmbeddedBotSettings
		"canary":  fresh.CanaryBot.Digest, // the canary bot package content + bot_config.py + EmbeddedBotSettings
	})

	// Update the config if it has changed. The transaction is needed to make sure
	// we always bump Fetched monotonically and that both ConfigBundle and
	// ConfigBundleRev entities are updated at the same time.
	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		lastRev := &configBundleRev{Key: configBundleRevKey(ctx)}
		switch err := datastore.Get(ctx, lastRev); {
		case err != nil:
			if !errors.Is(err, datastore.ErrNoSuchEntity) {
				return errors.Annotate(err, "failed to fetch the latest processed revision from datastore").Err()
			}
			logging.Infof(ctx, "First config import ever at rev %s", fresh.Revision)
			fresh.Touched = clock.Now(ctx).UTC().Truncate(time.Millisecond)
			fresh.Fetched = fresh.Touched

		case lastRev.Digest == fresh.Digest && lastRev.Revision == fresh.Revision:
			logging.Infof(ctx, "Configs are already up-to-date at rev %s (processed %s ago, content first seen %s ago)",
				lastRev.Revision,
				clock.Since(ctx, lastRev.Touched).Truncate(time.Second),
				clock.Since(ctx, lastRev.Fetched).Truncate(time.Second),
			)
			return nil

		case lastRev.Digest == fresh.Digest:
			// We've got a new git revision, but it didn't actually change any configs
			// since the digest is the same. Bump only Touched, but not Fetched.
			fresh.Touched = clock.Now(ctx).UTC().Truncate(time.Millisecond)
			fresh.Fetched = lastRev.Fetched
			logging.Infof(ctx, "Revision %s didn't change configs in a significant way (last change was %s ago)",
				fresh.Revision,
				clock.Since(ctx, lastRev.Fetched).Truncate(time.Second),
			)

		default:
			// We've got a new config either because the git state has changed or the
			// CIPD state has changed (or both). Log all changes.
			if lastRev.Revision != fresh.Revision {
				logging.Infof(ctx, "Config revision change %s => %s", lastRev.Revision, fresh.Revision)
			}
			lastRev.CanaryBot.logDiff(ctx, "Canary", fresh.CanaryBot)
			lastRev.StableBot.logDiff(ctx, "Stable", fresh.StableBot)
			// Bump both timestamps. Guarantee monotonic increase of Fetched even in
			// presence of clock skew, since the config cache mechanism relies on
			// this property.
			fresh.Touched = clock.Now(ctx).UTC().Truncate(time.Millisecond)
			fresh.Fetched = fresh.Touched
			if !fresh.Fetched.After(lastRev.Fetched) {
				fresh.Fetched = lastRev.Fetched.Add(time.Millisecond)
			}
		}

		return datastore.Put(ctx,
			&configBundle{
				VersionInfo: fresh,
				Key:         configBundleKey(ctx),
				Bundle:      bundle,
			},
			&configBundleRev{
				VersionInfo: fresh,
				Key:         configBundleRevKey(ctx),
			},
		)
	}, nil)
}

// defaultConfigs returns default config protos used on an "empty" server.
func defaultConfigs() *internalcfgpb.ConfigBundle {
	return &internalcfgpb.ConfigBundle{
		Revision: emptyRev,
		Digest:   emptyCfgDigest,
		Settings: withDefaultSettings(&configpb.SettingsCfg{}),
		Pools:    &configpb.PoolsCfg{},
		Bots: &configpb.BotsCfg{
			TrustedDimensions: []string{"pool"},
			BotGroup: []*configpb.BotGroup{
				{
					Dimensions: []string{"pool:unassigned"},
					Auth: []*configpb.BotAuth{
						{RequireLuciMachineToken: true, LogIfFailed: true},
					},
				},
			},
		},
		Scripts: map[string]string{},
	}
}

// parseAndValidateConfigs parses config files fetched from LUCI Config, if any.
func parseAndValidateConfigs(ctx context.Context, rev string, files map[string]config.Config) (*internalcfgpb.ConfigBundle, error) {
	bundle := defaultConfigs()
	bundle.Revision = rev

	// Parse and validate files individually.
	if err := parseAndValidate(ctx, files, settingsCfg, bundle.Settings, validateSettingsCfg); err != nil {
		return nil, err
	}
	if err := parseAndValidate(ctx, files, poolsCfg, bundle.Pools, validatePoolsCfg); err != nil {
		return nil, err
	}
	if err := parseAndValidate(ctx, files, botsCfg, bundle.Bots, validateBotsCfg); err != nil {
		return nil, err
	}

	// Make sure all referenced hook scripts are actually present. Collect them.
	for idx, group := range bundle.Bots.BotGroup {
		if group.BotConfigScript == "" {
			continue
		}
		if _, ok := bundle.Scripts[group.BotConfigScript]; ok {
			continue
		}
		body, ok := files["scripts/"+group.BotConfigScript]
		if !ok {
			return nil, errors.Reason("bot group #%d refers to undefined bot config script %q", idx+1, group.BotConfigScript).Err()
		}
		bundle.Scripts[group.BotConfigScript] = body.Content
	}

	// Derive the digest based exclusively on configs content, regardless of the
	// revision. The revision can change even if configs are unchanged. The digest
	// is used to know when configs are changing **for real**.
	visit := map[string]string{
		settingsCfg: files[settingsCfg].Content,
		poolsCfg:    files[poolsCfg].Content,
		botsCfg:     files[botsCfg].Content,
	}
	for script := range bundle.Scripts {
		visit["scripts/"+script] = files["scripts/"+script].Content
	}
	bundle.Digest = digest(visit)

	return bundle, nil
}

// parseAndValidate parses and validated one text proto config file.
func parseAndValidate[T any, TP interface {
	*T
	proto.Message
}](ctx context.Context,
	files map[string]config.Config,
	path string,
	cfg *T,
	validate func(ctx *validation.Context, t *T),
) error {
	// Parse it if it is present. Otherwise use the default value of `cfg`.
	if body := files[path]; body.Content != "" {
		if err := prototext.Unmarshal([]byte(body.Content), TP(cfg)); err != nil {
			return errors.Annotate(err, "%s", path).Err()
		}
	} else {
		logging.Warningf(ctx, "There's no %s config, using default", path)
	}
	// Pass through the validation, abort on fatal errors, allow warnings.
	valCtx := validation.Context{Context: ctx}
	validate(&valCtx, cfg)
	if err := valCtx.Finalize(); err != nil {
		var valErr *validation.Error
		if errors.As(err, &valErr) {
			blocking := valErr.WithSeverity(validation.Blocking)
			if blocking != nil {
				return errors.Annotate(blocking, "%s", path).Err()
			}
		} else {
			return errors.Annotate(err, "%s", path).Err()
		}
	}
	return nil
}

// digest calculates a digest of key value pairs.
func digest(pairs map[string]string) string {
	keys := make([]string, 0, len(pairs))
	for key := range pairs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "version 1\n")
	for _, key := range keys {
		_, _ = fmt.Fprintf(h, "%s\n%d\n%s\n", key, len(pairs[key]), pairs[key])
	}
	return base64.RawStdEncoding.EncodeToString(h.Sum(nil))
}

////////////////////////////////////////////////////////////////////////////////

// configBundle is an entity that stores latest configs as compressed protos.
type configBundle struct {
	VersionInfo

	Key    *datastore.Key              `gae:"$key"`
	Bundle *internalcfgpb.ConfigBundle `gae:",zstd"`

	_ datastore.PropertyMap `gae:"-,extra"`
}

// configBundleRev just stores the metadata for faster fetches.
type configBundleRev struct {
	VersionInfo

	Key *datastore.Key `gae:"$key"`

	_ datastore.PropertyMap `gae:"-,extra"`
}

// configBundleKey is a key of the singleton configBundle entity.
func configBundleKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, "ConfigBundle", "", 1, nil)
}

// configBundleRevKey is a key of the singleton configBundleRev entity.
func configBundleRevKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, "ConfigBundleRev", "", 1, configBundleKey(ctx))
}

// emptyVersionInfo is VersionInfo to use on an empty server with no configs.
func emptyVersionInfo() VersionInfo {
	return VersionInfo{
		Revision: emptyRev,
		Fetched:  time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC), // "distant" past
		Digest: digest(map[string]string{
			"configs": emptyCfgDigest,
			"stable":  "",
			"canary":  "",
		}),
	}
}

// fetchConfigBundleRev fetches the most recent configBundleRev.
//
// If there's no config in the datastore, returns some default value.
//
// It is a small entity with the information about the latest ingested config.
func fetchConfigBundleRev(ctx context.Context) (*configBundleRev, error) {
	rev := &configBundleRev{Key: configBundleRevKey(ctx)}
	switch err := datastore.Get(ctx, rev); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		rev.VersionInfo = emptyVersionInfo()
	case err != nil:
		return nil, errors.Annotate(err, "fetching configBundleRev").Tag(transient.Tag).Err()
	}
	return rev, nil
}

// fetchFromDatastore fetches the latest config from the datastore.
func fetchFromDatastore(ctx context.Context) (*Config, error) {
	bundle := &configBundle{Key: configBundleKey(ctx)}
	switch err := datastore.Get(ctx, bundle); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		bundle.VersionInfo = emptyVersionInfo()
		bundle.Bundle = defaultConfigs()
	case err != nil:
		return nil, errors.Annotate(err, "fetching configBundle").Err()
	}

	// Transform config protos into data structures optimized for config queries.
	// This should never really fail, since we store only validated configs. If
	// this fails, the entire service will eventually go offline since new
	// processes won't be able to load initial copy of the config (while old
	// processes will keep using last known good copies, until eventually they
	// all terminate).
	cfg, err := buildQueriableConfig(ctx, bundle)
	if err != nil {
		logging.Errorf(ctx,
			"Broken config in the datastore at rev %s (digest %s, fetched %s ago): %s",
			bundle.VersionInfo.Revision,
			bundle.VersionInfo.Digest,
			clock.Since(ctx, bundle.Fetched),
			err,
		)
		return nil, errors.Annotate(err, "broken config in the datastore").Err()
	}

	if bundle.VersionInfo.Revision != emptyRev {
		logging.Infof(ctx,
			"Loaded configs at rev %s (digest %s, first fetched %s ago)",
			bundle.VersionInfo.Revision,
			bundle.VersionInfo.Digest,
			clock.Since(ctx, bundle.Fetched),
		)
	} else {
		logging.Infof(ctx, "Loaded default config")
	}

	return cfg, nil
}

// buildQueriableConfig transforms config protos into data structures optimized
// for config queries.
func buildQueriableConfig(ctx context.Context, ent *configBundle) (*Config, error) {
	pools, err := newPoolsConfig(ent.Bundle.Pools)
	if err != nil {
		return nil, errors.Annotate(err, "bad pools.cfg").Err()
	}
	poolNames := make([]string, 0, len(pools))
	for name := range pools {
		poolNames = append(poolNames, name)
	}
	sort.Strings(poolNames)

	botGroups, err := newBotGroups(ent.Bundle.Bots, ent.Bundle.Scripts)
	if err != nil {
		return nil, errors.Annotate(err, "bad bots.cfg").Err()
	}

	settings := withDefaultSettings(ent.Bundle.Settings)
	traffic := make(map[string]*configpb.TrafficMigration_Route)
	if settings.TrafficMigration != nil {
		for _, r := range settings.TrafficMigration.Routes {
			traffic[r.Name] = r
		}
	}

	return &Config{
		VersionInfo: ent.VersionInfo,
		Refreshed:   clock.Now(ctx),
		DefaultCIPD: ent.Bundle.Pools.GetDefaultExternalServices().GetCipd(),
		settings:    settings,
		traffic:     traffic,
		poolMap:     pools,
		poolNames:   poolNames,
		botGroups:   botGroups,
	}, nil
}
