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

// Package settings contains global settings for starting a Config Server.
package settings

import (
	"context"
	"flag"
	"fmt"
	"regexp"

	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"

	"go.chromium.org/luci/config_service/internal/common"
)

var globalConfigLocRegex = regexp.MustCompile(`^gitiles\((.*),(.*),(.*)\)$`)

// GlobalConfigLoc is a setting to indicate the global config root location.
type GlobalConfigLoc struct {
	*cfgcommonpb.GitilesLocation
}

// RegisterFlags registers "-global-config-location" into command line flag set.
func (l *GlobalConfigLoc) RegisterFlags(fs *flag.FlagSet) {
	fs.TextVar(l, "global-config-location", &GlobalConfigLoc{}, text.Doc(`
		A string representation of luci.common.proto.config.GitilesLocation object
		which points to the root of service configs directory. The format should be
		'gitiles(<repo>,<ref>,<path>)'
	`))
}

// UnmarshalText unmarshal a textual representation of GlobalConfigLoc.
// Implements encoding.TextUnmarshaler.
func (l *GlobalConfigLoc) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		l.GitilesLocation = nil
		return nil
	}

	matches := globalConfigLocRegex.FindStringSubmatch(string(text))
	if len(matches) == 0 {
		return errors.Fmt(`does not match regex "%s"`, globalConfigLocRegex)
	}

	l.GitilesLocation = &cfgcommonpb.GitilesLocation{
		Repo: matches[1],
		Ref:  matches[2],
		Path: matches[3],
	}
	return nil
}

// MarshalText marshal itself into a textual form.
// Implements encoding.TextMarshaler.
func (l *GlobalConfigLoc) MarshalText() (text []byte, err error) {
	if l.GitilesLocation == nil {
		return nil, nil
	}
	return []byte(fmt.Sprintf("gitiles(%s,%s,%s)", l.Repo, l.Ref, l.Path)), nil
}

// Validate validates the GlobalConfigLoc
func (l *GlobalConfigLoc) Validate() error {
	return common.ValidateGitilesLocation(l.GitilesLocation)
}

var globalConfigLocKey = "holds the global config location"

// WithGlobalConfigLoc returns a context with the given config location.
func WithGlobalConfigLoc(ctx context.Context, location *cfgcommonpb.GitilesLocation) context.Context {
	return context.WithValue(ctx, &globalConfigLocKey, location)
}

// GetGlobalConfigLoc returns a GitilesLocation installed in the context.
func GetGlobalConfigLoc(ctx context.Context) *cfgcommonpb.GitilesLocation {
	return ctx.Value(&globalConfigLocKey).(*cfgcommonpb.GitilesLocation)
}
