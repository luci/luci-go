// Copyright 2016 The LUCI Authors.
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

package main

import (
	"fmt"
	"os/user"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/deploytool/api/deploy"
)

type cloudProjectVersion interface {
	fmt.Stringer

	isTainted() bool
}

type structuredCloudProjectVersion struct {
	// minorSourceVersion is the minorVersion parameter of the component's
	// origin Source.
	minorSourceVersion string
	// majorSourceVersion is the majorVersion parameter of the component's
	// origin Source.
	majorSourceVersion string

	// taintedUser, if not empty, indicates that this version is tainted and names
	// the user on whose behalf it was deployed.
	taintedUser string
}

func makeCloudProjectVersion(cp *layoutDeploymentCloudProject, src *layoutSource) (cloudProjectVersion, error) {
	return (cloudProjectVersionBuilder{}).build(cp, src)
}

type cloudProjectVersionBuilder struct {
	currentUser func() (string, error)
}

func (b cloudProjectVersionBuilder) build(cp *layoutDeploymentCloudProject, src *layoutSource) (cloudProjectVersion, error) {
	switch vs := cp.VersionScheme; vs {
	case deploy.Deployment_CloudProject_DEFAULT:
		cpv := structuredCloudProjectVersion{
			minorSourceVersion: cloudVersionStringNormalize(src.MinorVersion),
			majorSourceVersion: cloudVersionStringNormalize(src.MajorVersion),
		}

		if src.sg.Tainted {
			username, err := b.getCurrentUser()
			if err != nil {
				return nil, errors.Annotate(err, "could not get tained user").Err()
			}
			cpv.taintedUser = cloudVersionStringNormalize(username)
		}
		return &cpv, nil

	default:
		return nil, errors.Reason("unknown version scheme %v", vs).Err()
	}
}

func (b *cloudProjectVersionBuilder) getCurrentUser() (string, error) {
	if b.currentUser != nil {
		return b.currentUser()
	}

	// Default "currentUser" function uses "os.User".
	user, err := user.Current()
	if err != nil {
		return "", errors.Annotate(err, "could not get tained user").Err()
	}
	return user.Username, nil
}

func parseCloudProjectVersion(vs deploy.Deployment_CloudProject_VersionScheme, v string) (
	cloudProjectVersion, error) {
	var cpv structuredCloudProjectVersion
	switch vs {
	case deploy.Deployment_CloudProject_DEFAULT:
		parts := strings.Split(v, "-")

		switch len(parts) {
		case 4:
			// <min>-<maj>-tainted-<user>
			cpv.taintedUser = parts[3]
			fallthrough

		case 2:
			// <min>-<maj>
			cpv.minorSourceVersion = parts[0]
			cpv.majorSourceVersion = parts[1]
			return &cpv, nil

		default:
			return nil, errors.Reason("bad version %q for scheme %T", v, vs).Err()
		}

	default:
		return nil, errors.Reason("unknown version scheme %T", vs).Err()
	}
}

func (v *structuredCloudProjectVersion) String() string {
	var partsArray [5]string
	parts := partsArray[:0]

	parts = append(parts, []string{v.minorSourceVersion, v.majorSourceVersion}...)
	if tu := v.taintedUser; tu != "" {
		parts = append(parts, []string{"tainted", tu}...)
	}
	return strings.Join(parts, "-")
}

func (v *structuredCloudProjectVersion) isTainted() bool { return v.taintedUser != "" }

type stringCloudProjectVersion string

func makeStringCloudProjectVersion(v string) cloudProjectVersion {
	return stringCloudProjectVersion(cloudVersionStringNormalize(v))
}

func (v stringCloudProjectVersion) String() string  { return string(v) }
func (v stringCloudProjectVersion) isTainted() bool { return true }

// cloudVersionStringNormalize converts an input string into a cloud version-
// normalized string.
//
// Any character other than [A-Za-z0-9_] is converted to an underscore.
func cloudVersionStringNormalize(v string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, v)
}
