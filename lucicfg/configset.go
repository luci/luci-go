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
type ConfigSet struct {
	// Name is a name of this config set, e.g. "projects/something".
	//
	// It is used by LUCI Config to figure out how to validate files in the set.
	Name string

	// Data is files belonging to the config set.
	//
	//  Keys are slash-separated filenames, values are corresponding file bodies.
	Data map[string][]byte
}

// ValidationResult is what we get after validating a config set.
type ValidationResult struct {
	ConfigSet string               `json:"config_set"`          // a config set being validated
	Failed    bool                 `json:"failed"`              // true if the config is bad
	Messages  []*ValidationMessage `json:"messages"`            // errors, warnings, infos, etc.
	RPCError  string               `json:"rpc_error,omitempty"` // set if the RPC itself failed
}

// ConfigSetValidator is primarily implemented through config.Service, but can
// also be mocked in tests.
type ConfigSetValidator interface {
	// Validate sends the validation request to the service.
	//
	// Returns errors only on RPC errors. Actual validation errors are
	// communicated through []*ValidationMessage.
	Validate(ctx context.Context, req *ValidationRequest) ([]*ValidationMessage, error)
}

type remoteValidator struct {
	svc *config.Service
}

func (r remoteValidator) Validate(ctx context.Context, req *ValidationRequest) ([]*ValidationMessage, error) {
	resp, err := r.svc.ValidateConfig(req).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return resp.Messages, nil
}

// RemoteValidator returns ConfigSetValidator that makes RPCs to LUCI Config.
func RemoteValidator(svc *config.Service) ConfigSetValidator {
	return remoteValidator{svc}
}

// ReadConfigSet reads all regular files in the given directory (recursively)
// and returns them as a ConfigSet with given name.
func ReadConfigSet(dir, name string) (ConfigSet, error) {
	configs := map[string][]byte{}
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
		return ConfigSet{}, errors.Annotate(err, "failed to read config files").Err()
	}
	return ConfigSet{
		Name: name,
		Data: configs,
	}, nil
}

// Files returns a sorted list of file names in the config set.
func (cs ConfigSet) Files() []string {
	f := make([]string, 0, len(cs.Data))
	for k := range cs.Data {
		f = append(f, k)
	}
	sort.Strings(f)
	return f
}

// Validate sends the config set for validation to LUCI Config service.
//
// Returns ValidationResult with a list of validation message (errors, warnings,
// etc). The list of messages may be empty if the config set is 100% valid.
//
// If the RPC call itself failed, ValidationResult is still returned, but it has
// only ConfigSet and RPCError fields populated.
func (cs ConfigSet) Validate(ctx context.Context, val ConfigSetValidator) *ValidationResult {
	req := &ValidationRequest{
		ConfigSet: cs.Name,
		Files:     make([]*config.LuciConfigValidateConfigRequestMessageFile, len(cs.Data)),
	}

	logging.Infof(ctx, "Sending for validation to LUCI Config...")
	for idx, f := range cs.Files() {
		logging.Debugf(ctx, "  %s (%d bytes)", f, len(cs.Data[f]))
		req.Files[idx] = &config.LuciConfigValidateConfigRequestMessageFile{
			Path:    f,
			Content: base64.StdEncoding.EncodeToString(cs.Data[f]),
		}
	}

	messages, err := val.Validate(ctx, req)

	res := &ValidationResult{
		ConfigSet: cs.Name,
		Messages:  messages,
	}
	if err != nil {
		res.RPCError = err.Error()
		res.Failed = true
	}
	return res
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

	switch {
	case errs > 0:
		vr.Failed = true
		return errors.Reason("some files were invalid").Err()
	case warns > 0 && failOnWarnings:
		vr.Failed = true
		return errors.Reason("some files had validation warnings and -fail-on-warnings is set").Err()
	case vr.RPCError != "":
		vr.Failed = true
		return errors.Reason("failed to send RPC to LUCI Config - %s", vr.RPCError).Err()
	}

	vr.Failed = false
	return nil
}
