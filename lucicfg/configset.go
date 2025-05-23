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
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/klauspost/compress/gzip"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/config"
	configpb "go.chromium.org/luci/config_service/proto"
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

// AsOutput converts this config set into Output that have it at the given root
// path (usually ".").
func (cs ConfigSet) AsOutput(root string) Output {
	data := make(map[string]Datum, len(cs.Data))
	for k, v := range cs.Data {
		data[k] = BlobDatum(v)
	}
	return Output{
		Data:  data,
		Roots: map[string]string{cs.Name: root},
	}
}

// ValidationResult is what we get after validating a config set.
type ValidationResult struct {
	ConfigSet string              `json:"config_set"`          // a config set being validated
	Failed    bool                `json:"failed"`              // true if the config is bad
	Messages  []ValidationMessage `json:"messages"`            // errors, warnings, infos, etc.
	RPCError  string              `json:"rpc_error,omitempty"` // set if the RPC itself failed
}

// ValidationMessage is one validation message from the LUCI Config.
//
// It just wraps a proto, serializing it into JSON using JSONPB format using
// proto_names for fields. The result looks almost like the default json.Marshal
// serialization, except enum-valued fields (like Severity) use string enum
// names as values, not integers. The reason is that existing callers of lucicfg
// expect to see e.g. "WARNING" in the JSON, not "30".
type ValidationMessage struct {
	*config.ValidationResult_Message
}

// MarshalJSON implements json.Marshaler.
func (m ValidationMessage) MarshalJSON() ([]byte, error) {
	return (&protojson.MarshalOptions{UseProtoNames: true}).Marshal(m.ValidationResult_Message)
}

// ConfigSetValidator is primarily implemented through config.Service, but can
// also be mocked in tests.
type ConfigSetValidator interface {
	// Validate sends the validation request to the service.
	//
	// Returns errors only on RPC errors. Actual validation errors are
	// communicated through []*config.ValidationResult_Message.
	Validate(ctx context.Context, cs ConfigSet) ([]*config.ValidationResult_Message, error)
}

type remoteValidator struct {
	cfgClient configpb.ConfigsClient
}

// NewRemoteValidator makes a validator that validates configs via the LUCI
// Config service.
func NewRemoteValidator(conn *grpc.ClientConn) ConfigSetValidator {
	return &remoteValidator{
		cfgClient: configpb.NewConfigsClient(conn),
	}
}

// Validate implements ConfigSetValidator
func (r *remoteValidator) Validate(ctx context.Context, cs ConfigSet) ([]*config.ValidationResult_Message, error) {
	if len(cs.Data) == 0 {
		return nil, nil
	}
	validateReq := &configpb.ValidateConfigsRequest{
		ConfigSet:  cs.Name,
		FileHashes: make([]*configpb.ValidateConfigsRequest_FileHash, len(cs.Data)),
	}
	for i, file := range cs.Files() {
		content := cs.Data[file]
		h := sha256.New()
		h.Write(content)
		validateReq.FileHashes[i] = &configpb.ValidateConfigsRequest_FileHash{
			Path:   file,
			Sha256: hex.EncodeToString(h.Sum(nil)),
		}
	}
	res, err := r.cfgClient.ValidateConfigs(ctx, validateReq)
	switch fixInfo := findBadRequestFixInfo(err); {
	case fixInfo != nil:
		if err := uploadMissingFiles(ctx, cs, fixInfo.GetUploadFiles()); err != nil {
			return nil, err
		}
		validateReq.FileHashes = filterOutUnvalidatableFiles(ctx, validateReq.GetFileHashes(), fixInfo.GetUnvalidatableFiles())
		if len(validateReq.FileHashes) == 0 {
			logging.Debugf(ctx, "No config file need to be validated by LUCI Config")
			return nil, nil
		}
		switch res, err := r.cfgClient.ValidateConfigs(ctx, validateReq); { // now try again
		case err != nil:
			return nil, errors.Annotate(err, "failed to call LUCI Config").Err()
		default:
			return res.GetMessages(), nil
		}
	case err != nil:
		return nil, errors.Annotate(err, "failed to call LUCI Config").Err()
	default:
		return res.GetMessages(), nil
	}
}

func findBadRequestFixInfo(err error) *configpb.BadValidationRequestFixInfo {
	for _, detail := range status.Convert(err).Details() {
		switch t := detail.(type) {
		case *configpb.BadValidationRequestFixInfo:
			return t
		}
	}
	return nil
}

func filterOutUnvalidatableFiles(ctx context.Context,
	fileHashes []*configpb.ValidateConfigsRequest_FileHash,
	unvalidatableFiles []string) []*configpb.ValidateConfigsRequest_FileHash {
	if len(unvalidatableFiles) == 0 {
		return fileHashes
	}
	logging.Debugf(ctx, "No services can validate following files:\n  - %s", strings.Join(unvalidatableFiles, "\n  - "))
	unvalidatableFileSet := stringset.NewFromSlice(unvalidatableFiles...)
	ret := make([]*configpb.ValidateConfigsRequest_FileHash, 0, len(fileHashes))
	for _, fh := range fileHashes {
		if !unvalidatableFileSet.Has(fh.Path) {
			ret = append(ret, fh)
		}
	}
	return ret
}

func uploadMissingFiles(ctx context.Context, cs ConfigSet, uploadFiles []*configpb.BadValidationRequestFixInfo_UploadFile) error {
	if len(uploadFiles) == 0 {
		return nil
	}
	eg, ectx := errgroup.WithContext(ctx)
	for _, uf := range uploadFiles {
		eg.Go(func() error {
			logging.Debugf(ectx, "Uploading file %q for validation", uf.GetPath())
			start := clock.Now(ctx)

			pr, pw := io.Pipe()

			// Read and gzip in background, writing the compressed data into the pipe.
			// Buffer writes, since gzip writer outputs often and writes to a pipe are
			// slow-ish if unbuffered.
			done := make(chan struct{})
			go func() (err error) {
				defer func() {
					_ = pw.CloseWithError(err)
					close(done)
				}()
				bw := bufio.NewWriterSize(pw, 1024*512)
				zw := gzip.NewWriter(bw)
				if _, err := zw.Write(cs.Data[uf.GetPath()]); err != nil {
					_ = zw.Close()
					return errors.Annotate(err, "failed to write gzip data").Err()
				}
				if err := zw.Close(); err != nil {
					return errors.Annotate(err, "failed to close gzip writer").Err()
				}
				if err := bw.Flush(); err != nil {
					return errors.Annotate(err, "failed to flush writer").Err()
				}
				return nil
			}()
			defer func() {
				_ = pr.Close() // unblocks writes in the goroutine, if still blocked
				<-done         // waits for the goroutine to finish running
			}()

			// Read from the pipe and upload.
			req, err := http.NewRequestWithContext(ectx, http.MethodPut, uf.GetSignedUrl(), pr)
			if err != nil {
				return errors.Annotate(err, "failed to create http request to upload file %q", uf.GetPath()).Err()
			}
			req.Header.Add("Content-Encoding", "gzip")
			req.Header.Add("x-goog-content-length-range", fmt.Sprintf("0,%d", uf.GetMaxConfigSize()))

			switch res, err := http.DefaultClient.Do(req); {
			case err != nil:
				return errors.Annotate(err, "failed to execute http request to upload file %q", uf.GetPath()).Err()

			case res.StatusCode != http.StatusOK:
				defer func() { _ = res.Body.Close() }()
				body, err := io.ReadAll(res.Body)
				if err != nil {
					return errors.Annotate(err, "failed to read response body").Err()
				}
				return errors.Reason("failed to upload file %q;  got http response code: %d, body: %s", uf.GetPath(), res.StatusCode, string(body)).Err()

			default:
				defer func() { _ = res.Body.Close() }()
				logging.Debugf(ectx, "Successfully uploaded file %q for validation in %s", uf.GetPath(), clock.Since(ctx, start))
				return nil
			}
		})
	}
	return eg.Wait()
}

// ReadConfigSet reads all regular files in the given directory (recursively)
// and returns them as a ConfigSet with given name.
func ReadConfigSet(dir, name string) (ConfigSet, error) {
	configs := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.Type().IsRegular() {
			return err
		}
		content, err := os.ReadFile(p)
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
	logging.Infof(ctx, "Sending to LUCI Config for validation as config set %q:", cs.Name)
	for _, f := range cs.Files() {
		logging.Infof(ctx, "  %s (%s)", f, humanize.Bytes(uint64(len(cs.Data[f]))))
	}

	res := &ValidationResult{
		ConfigSet: cs.Name,
	}

	messages, err := val.Validate(ctx, cs)
	if len(messages) != 0 {
		res.Messages = make([]ValidationMessage, len(messages))
		for i, m := range messages {
			res.Messages[i] = ValidationMessage{
				ValidationResult_Message: m,
			}
		}
	}
	if err != nil {
		res.RPCError = err.Error()
		res.Failed = true
	}
	return res
}

// Format formats the validation result as a multi-line string
func (vr *ValidationResult) Format() string {
	buf := bytes.Buffer{}
	for _, msg := range vr.Messages {
		fmt.Fprintf(&buf, "%s: %s: %s: %s\n", msg.Severity, vr.ConfigSet, msg.Path, msg.Text)
	}
	return buf.String()
}

// OverallError is nil if the validation succeeded or non-nil if failed.
//
// Beware: mutates Failed field accordingly.
func (vr *ValidationResult) OverallError(failOnWarnings bool) error {
	errs, warns := 0, 0
	for _, msg := range vr.Messages {
		switch msg.Severity {
		case config.ValidationResult_WARNING:
			warns++
		case config.ValidationResult_ERROR, config.ValidationResult_CRITICAL:
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
