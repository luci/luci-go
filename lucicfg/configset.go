// Copyright 2018 The LUCI Authors.
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

package lucicfg

import (
	"context"
	"encoding/base64"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"

	config "go.chromium.org/luci/common/api/luci_config/config/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

// Alias some ridiculously long type names that we round-trip in the public API.
type (
	ValidationRequest = config.LuciConfigValidateConfigRequestMessage
	ValidationMessage = config.ComponentsConfigEndpointValidationMessage
)

// ConfigSet is an in-memory representation of a single config set.
//
// Keys are slash-separated filenames, values are corresponding file bodies.
type ConfigSet map[string][]byte

// ValidationResult is what we get after validating a config set.
type ValidationResult struct {
	Failed   bool                 `json:"failed"`   // true if the config is bad
	Messages []*ValidationMessage `json:"messages"` // errors, warning, infos, etc.
}

// ConfigSetValidator is primarily implemented through config.Service, but can
// also be mocked in tests.
type ConfigSetValidator interface {
	// Validate sends the validation request to the service.
	Validate(ctx context.Context, req *ValidationRequest) (*ValidationResult, error)
}

type remoteValidator struct {
	svc *config.Service
}

func (r remoteValidator) Validate(ctx context.Context, req *ValidationRequest) (*ValidationResult, error) {
	resp, err := r.svc.ValidateConfig(req).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return &ValidationResult{Messages: resp.Messages}, nil
}

// RemoteValidator returns ConfigSetValidator that makes RPCs to LUCI Config.
func RemoteValidator(svc *config.Service) ConfigSetValidator {
	return remoteValidator{svc}
}

// ReadConfigSet reads all regular files in the given directory (recursively)
// and returns them as a ConfigSet.
func ReadConfigSet(dir string) (ConfigSet, error) {
	configs := ConfigSet{}
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || !info.Mode().IsRegular() {
			return err
		}
		content, err := ioutil.ReadFile(p)
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		configs[filepath.ToSlash(relPath)] = content
		return nil
	})
	if err != nil {
		return nil, errors.Annotate(err, "failed to read config files").Err()
	}
	return configs, nil
}

// Files returns a sorted list of file names in the config set.
func (cs ConfigSet) Files() []string {
	f := make([]string, 0, len(cs))
	for k := range cs {
		f = append(f, k)
	}
	sort.Strings(f)
	return f
}

// Validate sends the config set for validation to LUCI Config service.
//
// 'name' is a name of this config set from LUCI Config point of view, e.g.
// "projects/<something>" or "services/<something>". It tells LUCI Config how
// to validate files in the set.
//
// Returns an error only if the validation call itself failed (e.g. LUCI Config
// was unreachable). Otherwise returns ValidationResult with a list of
// validation message (errors, warnings, etc). The list of messages may be empty
// if the config set is 100% valid.
func (cs ConfigSet) Validate(ctx context.Context, name string, val ConfigSetValidator) (*ValidationResult, error) {
	req := &ValidationRequest{
		ConfigSet: name,
		Files:     make([]*config.LuciConfigValidateConfigRequestMessageFile, len(cs)),
	}

	logging.Infof(ctx, "Sending for validation to LUCI Config...")
	for idx, f := range cs.Files() {
		logging.Debugf(ctx, "  %s (%d bytes)", f, len(cs[f]))
		req.Files[idx] = &config.LuciConfigValidateConfigRequestMessageFile{
			Path:    f,
			Content: base64.StdEncoding.EncodeToString(cs[f]),
		}
	}

	res, err := val.Validate(ctx, req)
	if err != nil {
		return nil, errors.Annotate(err, "error validating configs").Err()
	}
	return res, nil
}

// Log all messages in the result to the logger at an appropriate logging level.
func (vr *ValidationResult) Log(ctx context.Context) {
	for _, msg := range vr.Messages {
		lvl := logging.Info
		switch msg.Severity {
		case "WARNING":
			lvl = logging.Warning
		case "ERROR", "CRITICAL":
			lvl = logging.Error
		}
		logging.Logf(ctx, lvl, "%s: %s", msg.Path, msg.Text)
	}
}

// OverallError is nil if the validation succeeded or non-nil if failed.
//
// Beware: mutates Failed field accordingly.
func (vr *ValidationResult) OverallError(failOnWarnings bool) error {
	errs, warns := 0, 0
	for _, msg := range vr.Messages {
		switch msg.Severity {
		case "WARNING":
			warns++
		case "ERROR", "CRITICAL":
			errs++
		}
	}

	if errs > 0 {
		vr.Failed = true
		return errors.Reason("some files were invalid").Err()
	}
	if warns > 0 && failOnWarnings {
		vr.Failed = true
		return errors.Reason("some files had validation warnings and -fail-on-warnings is set").Err()
	}

	vr.Failed = false
	return nil
}
