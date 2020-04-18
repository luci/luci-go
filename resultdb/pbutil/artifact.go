// Copyright 2020 The LUCI Authors.
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

package pbutil

import (
	"net/url"
	"regexp"

	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
	sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"
)

// artifactFormatVersion identifies the version of artifact encoding format we're using.
const (
	artifactFormatVersion = 1
)

var artifactNameRe = regexp.MustCompile("^[[:word:]]([[:print:]]{0,254}[[:word:]])?$")

// ValidateArtifactName returns a non-nil error if name is invalid.
func ValidateArtifactName(name string) error {
	return validateWithRe(artifactNameRe, name)
}

// ValidateArtifactFetchURL returns a non-nil error if rawurl is invalid.
func ValidateArtifactFetchURL(rawurl string) error {
	switch u, err := url.ParseRequestURI(rawurl); {
	case err != nil:
		return err
	// ParseRequestURI un-capitalizes all the letters of Scheme.
	case u.Scheme != "https":
		return errors.Reason("the URL scheme is not HTTPS").Err()
	case u.Host == "":
		return errors.Reason("missing host").Err()
	}
	return nil
}

// ValidateArtifact returns a non-nil error if art is invalid.
func ValidateArtifact(art *pb.Artifact) (err error) {
	ec := checker{&err}
	switch {
	case art == nil:
		return unspecified()
	case ec.isErr(ValidateArtifactName(art.Name), "name"):
	case ec.isErr(ValidateArtifactFetchURL(art.FetchUrl), "fetch_url"):
		// skip `FetchUrlExpiration`
		// skip `ContentType`
		// skip `Size`
	}
	return err
}

// ValidateSinkArtifact returns a non-nil error if art is invalid.
func ValidateSinkArtifact(art *sinkpb.Artifact) error {
	if art.GetFilePath() == "" && art.GetContents() == nil {
		return errors.Reason("body: either file_path or contents must be provided").Err()
	}
	// TODO(1017288) - validate content_type
	// skip `ContentType`
	return nil
}

// ValidateSinkArtifacts returns a non-nil error if any element of arts is invalid.
func ValidateSinkArtifacts(arts map[string]*sinkpb.Artifact) error {
	for name, art := range arts {
		if art == nil {
			return errors.Reason("%s: %s", name, unspecified()).Err()
		}
		// the name should be a valid pb.Artifact.Name
		if err := ValidateArtifactName(name); err != nil {
			return errors.Annotate(err, "%s", name).Err()
		}
		if err := ValidateSinkArtifact(art); err != nil {
			return errors.Annotate(err, "%s", name).Err()
		}
	}
	return nil
}
