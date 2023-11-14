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
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/dustin/go-humanize"
	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zlib"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	legacy_config "go.chromium.org/luci/common/api/luci_config/config/v1"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/sync/parallel"
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
	ConfigSet string                             `json:"config_set"`          // a config set being validated
	Failed    bool                               `json:"failed"`              // true if the config is bad
	Messages  []*config.ValidationResult_Message `json:"messages"`            // errors, warnings, infos, etc.
	RPCError  string                             `json:"rpc_error,omitempty"` // set if the RPC itself failed
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
		uf := uf
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

type legacyRemoteValidator struct {
	validateConfig        func(context.Context, *legacy_config.LuciConfigValidateConfigRequestMessage) (*legacy_config.LuciConfigValidateConfigResponseMessage, error)
	requestSizeLimitBytes int64
}

func (r *legacyRemoteValidator) Validate(ctx context.Context, cs ConfigSet) ([]*config.ValidationResult_Message, error) {
	// Sort by size, smaller first, to group small files in a single request.
	files := make([]*legacy_config.LuciConfigValidateConfigRequestMessageFile, 0, len(cs.Data))
	for path, content := range cs.Data {
		files = append(files, &legacy_config.LuciConfigValidateConfigRequestMessageFile{
			Path:    path,
			Content: base64.StdEncoding.EncodeToString(content),
		})
	}
	sort.Slice(files, func(i, j int) bool {
		if len(files[i].Content) == len(files[j].Content) {
			return strings.Compare(files[i].Path, files[j].Path) < 0
		}
		return len(files[i].Content) < len(files[j].Content)
	})

	// Split all files into a bunch of smallish validation requests to avoid
	// hitting 32MB request size limit.
	var (
		requests []*legacy_config.LuciConfigValidateConfigRequestMessage
		curFiles []*legacy_config.LuciConfigValidateConfigRequestMessageFile
		curSize  int64
	)
	flush := func() {
		if len(curFiles) > 0 {
			requests = append(requests, &legacy_config.LuciConfigValidateConfigRequestMessage{
				ConfigSet: cs.Name,
				Files:     curFiles,
			})
		}
		curFiles = nil
		curSize = 0
	}
	for _, f := range files {
		switch contentSize := int64(len(f.Content)); {
		case contentSize > r.requestSizeLimitBytes:
			return nil, errors.Reason("the size of file %q is %s that is exceeding the limit of %s", f.Path, humanize.Bytes(uint64(contentSize)), humanize.Bytes(uint64(r.requestSizeLimitBytes))).Err()
		case curSize+contentSize > r.requestSizeLimitBytes:
			flush()
			fallthrough
		default:
			curFiles = append(curFiles, f)
			curSize += int64(len(f.Content))
		}
	}
	flush()

	var (
		lock     sync.Mutex
		messages []*config.ValidationResult_Message
	)

	// Execute all requests in parallel.
	err := parallel.FanOutIn(func(gen chan<- func() error) {
		for _, req := range requests {
			req := req
			gen <- func() error {
				resp, err := r.validateConfig(ctx, req)
				if resp != nil {
					lock.Lock()
					for _, msg := range resp.Messages {
						if val, ok := config.ValidationResult_Severity_value[strings.ToUpper(msg.Severity)]; ok {
							messages = append(messages, &config.ValidationResult_Message{
								Path:     msg.Path,
								Severity: config.ValidationResult_Severity(val),
								Text:     msg.Text,
							})
						} else {
							logging.Warningf(ctx, "unknown severity %q; full msg: %+v", msg.Severity, msg)
						}
					}
					lock.Unlock()
				}
				return err
			}
		}
	})

	// Sort messages by path for determinism.
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Path < messages[j].Path
	})

	return messages, err
}

// LegacyRemoteValidator returns ConfigSetValidator that makes RPCs to legacy
// LUCI Config service.
func LegacyRemoteValidator(client *http.Client, host string) ConfigSetValidator {
	validateURL := fmt.Sprintf("https://%s/_ah/api/config/v1/validate-config", host)
	return &legacyRemoteValidator{
		// 160 MiB is picked because compression is done before sending the final
		// request and the real request size limit is 32 MiB. Since config is
		// highly repetitive content, it should easily achieve 5:1 compression
		// ratio.
		requestSizeLimitBytes: 160 * 1024 * 1024,
		validateConfig: func(ctx context.Context, req *legacy_config.LuciConfigValidateConfigRequestMessage) (*legacy_config.LuciConfigValidateConfigResponseMessage, error) {

			debug := make([]string, len(req.Files))
			for i, f := range req.Files {
				debug[i] = fmt.Sprintf("%s (%s)", f.Path, humanize.Bytes(uint64(len(f.Content))))
			}
			logging.Debugf(ctx, "Sending request to %s to validate %d files: %s",
				validateURL,
				len(req.Files),
				strings.Join(debug, ", "),
			)

			var body bytes.Buffer
			zlibWriter := zlib.NewWriter(&body)
			if err := json.NewEncoder(zlibWriter).Encode(req); err != nil {
				return nil, errors.Annotate(err, "failed to encode the request").Err()
			}
			if err := zlibWriter.Close(); err != nil {
				return nil, errors.Annotate(err, "failed to close the zlib stream").Err()
			}
			httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, validateURL, &body)
			if err != nil {
				return nil, errors.Annotate(err, "failed to create a new request").Err()
			}
			httpReq.Header.Add("Content-Type", `application/json-zlib`)
			httpReq.Header.Add("User-Agent", UserAgent)

			res, err := client.Do(httpReq)
			if err != nil {
				return nil, errors.Annotate(err, "failed to execute HTTP request").Err()
			}
			defer func() { _ = res.Body.Close() }()
			if res.StatusCode < 200 || res.StatusCode > 299 {
				return nil, googleapi.CheckResponse(res)
			}
			ret := &legacy_config.LuciConfigValidateConfigResponseMessage{
				ServerResponse: googleapi.ServerResponse{
					Header:         res.Header,
					HTTPStatusCode: res.StatusCode,
				},
			}
			if err := json.NewDecoder(res.Body).Decode(&ret); err != nil {
				return nil, err
			}
			return ret, nil
		},
	}
}

// ReadConfigSet reads all regular files in the given directory (recursively)
// and returns them as a ConfigSet with given name.
func ReadConfigSet(dir, name string) (ConfigSet, error) {
	configs := map[string][]byte{}
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || !info.Mode().IsRegular() {
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

	messages, err := val.Validate(ctx, cs)
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
