// Copyright 2024 The LUCI Authors.
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

// Package pbutil contains methods for manipulating protobufs.
package pbutil

import (
	"fmt"
	"regexp"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/tree_status/proto/v1"
)

const (
	// TreeIDExpression is a partial regular expression that validates tree identifiers.
	TreeIDExpression = `[a-z](?:[a-z0-9-]{0,61}[a-z0-9])?`
	// StatusIDExpression is a partial regular expression that validates status identifiers.
	StatusIDExpression = `[0-9a-f]{32}`
	// BuilderNameExpression is a partial regular expression that validates builder name.
	BuilderNameExpression = `projects/([a-z0-9\-_]{1,40})/buckets/([a-z0-9\-_.]{1,100})/builders/([a-zA-Z0-9\-_.\(\) ]{1,128})`
	// ProjectExpression is a partial regular expression that validates LUCI Project.
	ProjectExpression = `[a-z0-9\-]{1,40}`
	// TreeNameExpression is a partial regular expression that validates tree name.
	TreeNameExpression = `trees/(` + TreeIDExpression + `)`
)

func ValidateTreeID(treeName string) error {
	if treeName == "" {
		return errors.New("must be specified")
	}
	var treeIDRE = regexp.MustCompile(`^` + TreeIDExpression + `$`)
	if !treeIDRE.MatchString(treeName) {
		return errors.Fmt("expected format: %s", treeIDRE)
	}
	return nil
}

func ValidateStatusID(id string) error {
	if id == "" {
		return errors.New("must be specified")
	}
	var statusIDRE = regexp.MustCompile(`^` + StatusIDExpression + `$`)
	if !statusIDRE.MatchString(id) {
		return errors.Fmt("expected format: %s", statusIDRE)
	}
	return nil
}

func ValidateGeneralStatus(state pb.GeneralState) error {
	if state == pb.GeneralState_GENERAL_STATE_UNSPECIFIED {
		return errors.New("must be specified")
	}
	if _, ok := pb.GeneralState_name[int32(state)]; !ok {
		return errors.New("invalid enum value")
	}
	return nil
}

func ValidateMessage(message string) error {
	if message == "" {
		return errors.New("must be specified")
	}
	if len(message) > 1024 {
		return errors.New("longer than 1024 bytes")
	}
	if !utf8.ValidString(message) {
		return errors.New("not a valid utf8 string")
	}
	if !norm.NFC.IsNormalString(message) {
		return errors.New("not in unicode normalized form C")
	}
	for i, rune := range message {
		if !unicode.IsPrint(rune) {
			return fmt.Errorf("non-printable rune %+q at byte index %d", rune, i)
		}
	}
	return nil
}

func ValidateClosingBuilderName(name string) error {
	// We allow closing without specifying builder name.
	if name == "" {
		return nil
	}
	var builderNameRE = regexp.MustCompile(`^` + BuilderNameExpression + `$`)
	if !builderNameRE.MatchString(name) {
		return errors.Fmt("expected format: %s", builderNameRE)
	}
	return nil
}

func ValidateProject(project string) error {
	if project == "" {
		return errors.New("must be specified")
	}
	var projectRE = regexp.MustCompile(`^` + ProjectExpression + `$`)
	if !projectRE.MatchString(project) {
		return errors.Fmt("expected format: %s", projectRE)
	}
	return nil
}

func ParseTreeName(treeName string) (treeID string, err error) {
	if treeName == "" {
		return "", errors.New("must be specified")
	}
	var treeNameRE = regexp.MustCompile(`^` + TreeNameExpression + `$`)
	match := treeNameRE.FindStringSubmatch(treeName)
	if match == nil {
		return "", errors.Fmt("expected format: %s", treeNameRE)
	}
	return match[1], nil
}
