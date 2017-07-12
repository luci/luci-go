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

package deploy

import (
	"fmt"
	"strings"
	"time"

	"github.com/luci/luci-go/common/errors"
)

// AppYAMLString returns the GAE app.yaml string for this Duration.
func (d *Duration) AppYAMLString() string { return strings.Join(d.appYAMLStringParts(), " ") }

func (d *Duration) appYAMLStringParts() (parts []string) {
	if d != nil {
		parts = []string{fmt.Sprintf("%d%s", d.Value, d.Unit.appYAMLSuffix())}
		for _, plus := range d.Plus {
			parts = append(parts, plus.appYAMLStringParts()...)
		}
	}
	return
}

// Duration returns the time.Duration that this Duration describes.
func (d *Duration) Duration() time.Duration {
	if d == nil {
		return 0
	}

	dv := time.Duration(d.Value) * d.Unit.durationUnit()
	for _, plus := range d.Plus {
		dv += plus.Duration()
	}
	return dv
}

func (u Duration_Unit) appYAMLSuffix() string {
	switch u {
	case Duration_MILLISECONDS:
		return "ms"
	case Duration_SECONDS:
		return "s"
	case Duration_MINUTES:
		return "m"
	case Duration_HOURS:
		return "h"
	case Duration_DAYS:
		return "d"
	default:
		panic(fmt.Errorf("unknown Duration.Unit: %v", u))
	}
}

func (u Duration_Unit) durationUnit() time.Duration {
	switch u {
	case Duration_MILLISECONDS:
		return time.Millisecond
	case Duration_SECONDS:
		return time.Second
	case Duration_MINUTES:
		return time.Minute
	case Duration_HOURS:
		return time.Hour
	case Duration_DAYS:
		return 24 * time.Hour
	default:
		panic(fmt.Errorf("unknown Duration.Unit: %v", u))
	}
}

// AppYAMLString returns the "app.yaml" string for this instance class.
//
// The supplied prefix is prepended to the instance class string. This prefix
// will differ based on the type of AppEngine module that the instance class
// is describing (e.g., for "F_1", the prefix is "F_").
func (v AppEngineModule_ClassicParams_InstanceClass) AppYAMLString(prefix string) string {
	var name string
	switch v {
	case AppEngineModule_ClassicParams_IC_DEFAULT:
		return ""

	case AppEngineModule_ClassicParams_IC_1:
		name = "1"
	case AppEngineModule_ClassicParams_IC_2:
		name = "2"
	case AppEngineModule_ClassicParams_IC_4:
		name = "4"
	case AppEngineModule_ClassicParams_IC_4_1G:
		name = "4_1G"
	case AppEngineModule_ClassicParams_IC_8:
		name = "8"
	default:
		panic(fmt.Errorf("unknown InstanceClass: %v", v))
	}

	return prefix + name
}

// AppYAMLString returns the "app.yaml" string for this option.
func (v AppEngineModule_Handler_LoginOption) AppYAMLString() string {
	switch v {
	case AppEngineModule_Handler_LOGIN_OPTIONAL:
		return "optional"
	case AppEngineModule_Handler_LOGIN_REQUIRED:
		return "required"
	case AppEngineModule_Handler_LOGIN_ADMIN:
		return "admin"
	default:
		panic(fmt.Errorf("unknown handler login option %q", v))
	}
}

// AppYAMLString returns the "app.yaml" string for this option.
func (v AppEngineModule_Handler_SecureOption) AppYAMLString() string {
	switch v {
	case AppEngineModule_Handler_SECURE_OPTIONAL:
		return "optional"
	case AppEngineModule_Handler_SECURE_NEVER:
		return "never"
	case AppEngineModule_Handler_SECURE_ALWAYS:
		return "always"
	default:
		panic(fmt.Errorf("unknown handler secure option %q", v))
	}
}

// AppYAMLString returns the "index.yaml" string for this option.
func (v AppEngineResources_Index_Direction) AppYAMLString() string {
	switch v {
	case AppEngineResources_Index_ASCENDING:
		return "asc"
	case AppEngineResources_Index_DESCENDING:
		return "desc"
	default:
		panic(fmt.Errorf("unknown index direction %q", v))
	}
}

// IndexDirectionFromAppYAMLString returns the
// AppEngineResources_Index_Direction value associated with the given
// "index.yaml" strnig value.
//
// This is a reverse of the AppEngineResources_Index_Direction's AppYAMLString.
//
// If the direction value is not recognized, an error will be returned.
func IndexDirectionFromAppYAMLString(v string) (AppEngineResources_Index_Direction, error) {
	switch v {
	case "asc", "":
		return AppEngineResources_Index_ASCENDING, nil

	case "desc":
		return AppEngineResources_Index_DESCENDING, nil

	default:
		return 0, errors.Reason("invalid index direction %q", v).Err()
	}
}

// KubeString returns the Kubernetes "restartPolicy" field string for the
// enumeration value.
func (v KubernetesPod_RestartPolicy) KubeString() string {
	switch v {
	case KubernetesPod_RESTART_ALWAYS:
		return "Always"
	case KubernetesPod_RESTART_ON_FAILURE:
		return "OnFailure"
	case KubernetesPod_RESTART_NEVER:
		return "Never"
	default:
		panic(fmt.Errorf("unknown restart policy (%v)", v))
	}
}

// KubeSuffix returns the Kubernetes configuration unit suffix for a given
// resource unit.
func (v KubernetesPod_Container_Resources_Unit) KubeSuffix() string {
	switch v {
	case KubernetesPod_Container_Resources_BYTE:
		return ""

	case KubernetesPod_Container_Resources_KILOBYTE:
		return "K"
	case KubernetesPod_Container_Resources_MEGABYTE:
		return "M"
	case KubernetesPod_Container_Resources_GIGABYTE:
		return "G"
	case KubernetesPod_Container_Resources_TERABYTE:
		return "T"
	case KubernetesPod_Container_Resources_PETABYTE:
		return "P"
	case KubernetesPod_Container_Resources_EXABYTE:
		return "E"

	case KubernetesPod_Container_Resources_KIBIBYTE:
		return "Ki"
	case KubernetesPod_Container_Resources_MEBIBYTE:
		return "Mi"
	case KubernetesPod_Container_Resources_GIBIBYTE:
		return "Gi"
	case KubernetesPod_Container_Resources_TEBIBYTE:
		return "Ti"
	case KubernetesPod_Container_Resources_PEBIBYTE:
		return "Pi"
	case KubernetesPod_Container_Resources_EXBIBYTE:
		return "Ei"

	default:
		panic(fmt.Errorf("unknown resource unit (%v)", v))
	}
}
