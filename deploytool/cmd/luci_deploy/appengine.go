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
	"io/ioutil"
	"path/filepath"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/deploytool/api/deploy"
	"gopkg.in/yaml.v2"
)

// gaeAppYAML is a YAML struct for an AppEngine "app.yaml".
type gaeAppYAML struct {
	Module        string      `yaml:"module,omitempty"`
	Runtime       string      `yaml:"runtime,omitempty"`
	ThreadSafe    *bool       `yaml:"threadsafe,omitempty"`
	APIVersion    interface{} `yaml:"api_version,omitempty"`
	VM            bool        `yaml:"vm,omitempty"`
	InstanceClass string      `yaml:"instance_class,omitempty"`

	AutomaticScaling *gaeAppAutomaticScalingParams `yaml:"automatic_scaling,omitempty"`
	BasicScaling     *gaeAppBasicScalingParams     `yaml:"basic_scaling,omitempty"`
	ManualScaling    *gaeAppManualScalingParams    `yaml:"manual_scaling,omitempty"`

	BetaSettings *gaeAppYAMLBetaSettings `yaml:"beta_settings,omitempty"`

	Handlers []*gaeAppYAMLHandler `yaml:"handlers,omitempty"`
}

// gaeAppAutomaticScalingParams is the combined set of scaling parameters for
// automatic scaling.
type gaeAppAutomaticScalingParams struct {
	MinIdleInstances      *int   `yaml:"min_idle_instances,omitempty"`
	MaxIdleInstances      *int   `yaml:"max_idle_instances,omitempty"`
	MinPendingLatency     string `yaml:"min_pending_latency,omitempty"`
	MaxPendingLatency     string `yaml:"max_pending_latency,omitempty"`
	MaxConcurrentRequests *int   `yaml:"max_concurrent_requests,omitempty"`
}

// gaeAppBasicScalingParams is the combined set of scaling parameters for
// basic scaling.
type gaeAppBasicScalingParams struct {
	IdleTimeout  string `yaml:"idle_timeout,omitempty"`
	MaxInstances int    `yaml:"max_instances"`
}

// gaeAppManualScalingParams is the combined set of scaling parameters for
// manual scaling.
type gaeAppManualScalingParams struct {
	Instances int `yaml:"instances"`
}

// gaeAppYAMLBuiltIns is the set of builtin options that can be enabled. Any
// enabled option will have the value, "on".
type gaeAppYAMLBuiltIns struct {
	AppStats  string `yaml:"appstats,omitempty"`
	RemoteAPI string `yaml:"remote_api,omitempty"`
	Deferred  string `yaml:"deferred,omitempty"`
}

type gaeLibrary struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version,omitempty"`
}

// gaeAppYAMLBetaSettings is a YAML struct for an AppEngine "app.yaml"
// beta settings.
type gaeAppYAMLBetaSettings struct {
	ServiceAccountScopes string `yaml:"service_account_scopes,omitempty"`
}

// gaeAppYAMLHAndler is a YAML struct for an AppEngine "app.yaml"
// handler entry.
type gaeAppYAMLHandler struct {
	URL    string `yaml:"url"`
	Script string `yaml:"script,omitempty"`

	Secure string `yaml:"secure,omitempty"`
	Login  string `yaml:"login,omitempty"`

	StaticDir   string `yaml:"static_dir,omitempty"`
	StaticFiles string `yaml:"static_files,omitempty"`
	Upload      string `yaml:"upload,omitempty"`
	Expiration  string `yaml:"expiration,omitempty"`

	HTTPHeaders map[string]string `yaml:"http_headers,omitempty"`
}

// gaeCronYAML is a YAML struct for an AppEngine "cron.yaml".
type gaeCronYAML struct {
	Cron []*gaeCronYAMLEntry `yaml:"cron,omitempty"`
}

// gaeCronYAMLEntry is a YAML struct for an AppEngine "cron.yaml" entry.
type gaeCronYAMLEntry struct {
	Description string `yaml:"description,omitempty"`
	URL         string `yaml:"url"`
	Schedule    string `yaml:"schedule"`
	Target      string `yaml:"target,omitempty"`
}

// gaeIndexYAML is a YAML struct for an AppEngine "index.yaml".
type gaeIndexYAML struct {
	Indexes []*gaeIndexYAMLEntry `yaml:"indexes,omitempty"`
}

// gaeIndexYAMLEntry is a YAML struct for an AppEngine "index.yaml" entry.
type gaeIndexYAMLEntry struct {
	Kind       string                       `yaml:"kind"`
	Ancestor   interface{}                  `yaml:"ancestor,omitempty"`
	Properties []*gaeIndexYAMLEntryProperty `yaml:"properties"`
}

// gaeIndexYAMLEntryProperty is a YAML struct for an AppEngine "index.yaml"
// entry's property.
type gaeIndexYAMLEntryProperty struct {
	Name      string `yaml:"name"`
	Direction string `yaml:"direction,omitempty"`
}

// gaeDispatchYAML is a YAML struct for an AppEngine "dispatch.yaml".
type gaeDispatchYAML struct {
	Dispatch []*gaeDispatchYAMLEntry `yaml:"dispatch,omitempty"`
}

// gaeDispatchYAMLEntry is a YAML struct for an AppEngine "dispatch.yaml" entry.
type gaeDispatchYAMLEntry struct {
	Module string `yaml:"module"`
	URL    string `yaml:"url"`
}

// gaeQueueYAML is a YAML struct for an AppEngine "queue.yaml".
type gaeQueueYAML struct {
	Queue []*gaeQueueYAMLEntry `yaml:"queue,omitempty"`
}

// gaeQueueYAMLEntry is a YAML struct for an AppEngine "queue.yaml" entry.
type gaeQueueYAMLEntry struct {
	Name string `yaml:"name"`
	Mode string `yaml:"mode"`

	Rate                  string `yaml:"rate,omitempty"`
	BucketSize            int    `yaml:"bucket_size,omitempty"`
	MaxConcurrentRequests int    `yaml:"max_concurrent_requests,omitempty"`
	Target                string `yaml:"target,omitempty"`

	RetryParameters *gaeQueueYAMLEntryRetryParameters `yaml:"retry_parameters,omitempty"`
}

// gaeQueueYAMLEntryRetryParameters is a YAML struct for an AppEngine
// "queue.yaml" entry push queue retry parameters.
type gaeQueueYAMLEntryRetryParameters struct {
	TaskAgeLimit      string `yaml:"task_age_limit,omitempty"`
	MinBackoffSeconds int    `yaml:"min_backoff_seconds,omitempty"`
	MaxBackoffSeconds int    `yaml:"max_backoff_seconds,omitempty"`
	MaxDoublings      int    `yaml:"max_doublings,omitempty"`
}

func gaeBuildAppYAML(aem *deploy.AppEngineModule, staticMap map[*deploy.BuildPath]string) (*gaeAppYAML, error) {
	appYAML := gaeAppYAML{
		Module: aem.ModuleName,
	}

	var (
		defaultScript = ""
		isStub        = false
	)

	switch aem.GetRuntime().(type) {
	case *deploy.AppEngineModule_GoModule_:
		appYAML.Runtime = "go"

		if aem.GetManagedVm() != nil {
			appYAML.APIVersion = 1
			appYAML.VM = true
		} else {
			appYAML.APIVersion = "go1"
		}
		defaultScript = "_go_app"

	case *deploy.AppEngineModule_StaticModule_:
		// A static module presents itself as an empty Python AppEngine module. This
		// doesn't actually need any Python code to support it.
		appYAML.Runtime = "python27"
		appYAML.APIVersion = "1"
		threadSafe := true
		appYAML.ThreadSafe = &threadSafe
		isStub = true

	default:
		return nil, errors.Reason("unsupported runtime %q", aem.Runtime).Err()
	}

	// Classic / Managed VM Properties
	switch p := aem.GetParams().(type) {
	case *deploy.AppEngineModule_Classic:
		var icPrefix string
		switch sc := p.Classic.GetScaling().(type) {
		case nil:
			// (Automatic, default).
			icPrefix = "F"

		case *deploy.AppEngineModule_ClassicParams_AutomaticScaling_:
			as := sc.AutomaticScaling
			appYAML.AutomaticScaling = &gaeAppAutomaticScalingParams{
				MinIdleInstances:      intPtrOrNilIfZero(as.MinIdleInstances),
				MaxIdleInstances:      intPtrOrNilIfZero(as.MaxIdleInstances),
				MinPendingLatency:     as.MinPendingLatency.AppYAMLString(),
				MaxPendingLatency:     as.MaxPendingLatency.AppYAMLString(),
				MaxConcurrentRequests: intPtrOrNilIfZero(as.MaxConcurrentRequests),
			}
			icPrefix = "F"

		case *deploy.AppEngineModule_ClassicParams_BasicScaling_:
			bs := sc.BasicScaling
			appYAML.BasicScaling = &gaeAppBasicScalingParams{
				IdleTimeout:  bs.IdleTimeout.AppYAMLString(),
				MaxInstances: int(bs.MaxInstances),
			}
			icPrefix = "B"

		case *deploy.AppEngineModule_ClassicParams_ManualScaling_:
			ms := sc.ManualScaling
			appYAML.ManualScaling = &gaeAppManualScalingParams{
				Instances: int(ms.Instances),
			}
			icPrefix = "B"

		default:
			return nil, errors.Reason("unknown scaling type %T", sc).Err()
		}
		appYAML.InstanceClass = p.Classic.InstanceClass.AppYAMLString(icPrefix)

	case *deploy.AppEngineModule_ManagedVm:
		if scopes := p.ManagedVm.Scopes; len(scopes) > 0 {
			appYAML.BetaSettings = &gaeAppYAMLBetaSettings{
				ServiceAccountScopes: strings.Join(scopes, ","),
			}
		}
	}

	if handlerSet := aem.Handlers; handlerSet != nil {
		appYAML.Handlers = make([]*gaeAppYAMLHandler, len(handlerSet.Handler))
		for i, handler := range handlerSet.Handler {
			entry := gaeAppYAMLHandler{
				URL:    handler.Url,
				Secure: handler.Secure.AppYAMLString(),
				Login:  handler.Login.AppYAMLString(),
			}

			// Fill in static dir/file paths.
			switch t := handler.GetContent().(type) {
			case nil:
				entry.Script = defaultScript

			case *deploy.AppEngineModule_Handler_Script:
				entry.Script = t.Script
				if entry.Script == "" {
					entry.Script = defaultScript
				}

			case *deploy.AppEngineModule_Handler_StaticBuildDir:
				entry.StaticDir = staticMap[t.StaticBuildDir]

			case *deploy.AppEngineModule_Handler_StaticFiles_:
				sf := t.StaticFiles

				relDir := staticMap[sf.GetBuild()]
				entry.Upload = filepath.Join(relDir, sf.Upload)
				entry.StaticFiles = filepath.Join(relDir, sf.UrlMap)

			default:
				return nil, errors.Reason("don't know how to handle content %T", t).
					InternalReason("url(%q)", handler.Url).Err()
			}

			if entry.Script != "" && isStub {
				return nil, errors.Reason("stub module cannot have entry script").Err()
			}

			appYAML.Handlers[i] = &entry
		}
	}

	return &appYAML, nil
}

func gaeBuildCronYAML(cp *layoutDeploymentCloudProject) *gaeCronYAML {
	var cronYAML gaeCronYAML

	for _, m := range cp.appEngineModules {
		for _, cron := range m.resources.Cron {
			cronYAML.Cron = append(cronYAML.Cron, &gaeCronYAMLEntry{
				Description: cron.Description,
				URL:         cron.Url,
				Schedule:    cron.Schedule,
				Target:      m.ModuleName,
			})
		}
	}
	return &cronYAML
}

func gaeBuildIndexYAML(cp *layoutDeploymentCloudProject) *gaeIndexYAML {
	var indexYAML gaeIndexYAML

	for _, index := range cp.resources.Index {
		entry := gaeIndexYAMLEntry{
			Kind:       index.Kind,
			Properties: make([]*gaeIndexYAMLEntryProperty, len(index.Property)),
			Ancestor:   index.Ancestor,
		}
		for i, prop := range index.Property {
			entry.Properties[i] = &gaeIndexYAMLEntryProperty{
				Name:      prop.Name,
				Direction: prop.Direction.AppYAMLString(),
			}
		}
		indexYAML.Indexes = append(indexYAML.Indexes, &entry)
	}
	return &indexYAML
}

func gaeBuildDispatchYAML(cp *layoutDeploymentCloudProject) (*gaeDispatchYAML, error) {
	var dispatchYAML gaeDispatchYAML

	for _, m := range cp.appEngineModules {
		for _, url := range m.resources.Dispatch {
			dispatchYAML.Dispatch = append(dispatchYAML.Dispatch, &gaeDispatchYAMLEntry{
				Module: m.ModuleName,
				URL:    url,
			})
		}
	}
	if count := len(dispatchYAML.Dispatch); count > 10 {
		return nil, errors.Reason("dispatch file has more than 10 routing rules (%d)", count).Err()
	}
	return &dispatchYAML, nil
}

func gaeBuildQueueYAML(cp *layoutDeploymentCloudProject) (*gaeQueueYAML, error) {
	var queueYAML gaeQueueYAML

	for _, m := range cp.appEngineModules {
		for _, queue := range m.resources.TaskQueue {
			entry := gaeQueueYAMLEntry{
				Name: queue.Name,
			}
			switch t := queue.GetType().(type) {
			case *deploy.AppEngineResources_TaskQueue_Push_:
				push := t.Push

				entry.Target = m.ModuleName
				entry.Mode = "push"
				entry.Rate = push.Rate
				entry.BucketSize = int(push.BucketSize)
				entry.MaxConcurrentRequests = int(push.MaxConcurrentRequests)
				entry.RetryParameters = &gaeQueueYAMLEntryRetryParameters{
					TaskAgeLimit:      push.RetryTaskAgeLimit,
					MinBackoffSeconds: int(push.RetryMinBackoffSeconds),
					MaxBackoffSeconds: int(push.RetryMaxBackoffSeconds),
					MaxDoublings:      int(push.RetryMaxDoublings),
				}

			default:
				return nil, errors.Reason("unknown task queue type %T", t).Err()
			}

			queueYAML.Queue = append(queueYAML.Queue, &entry)
		}
	}
	return &queueYAML, nil
}

// loadIndexYAMLResource loads an AppEngine "index.yaml" file from the specified
// filesystem path and translates it into AppEngineResources Index entries.
func loadIndexYAMLResource(path string) (*deploy.AppEngineResources, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.Annotate(err, "failed to load [%s]", path).Err()
	}

	var indexYAML gaeIndexYAML
	if err := yaml.Unmarshal(data, &indexYAML); err != nil {
		return nil, errors.Annotate(err, "could not load 'index.yaml' from [%s]", path).Err()
	}

	var res deploy.AppEngineResources
	if len(indexYAML.Indexes) > 0 {
		res.Index = make([]*deploy.AppEngineResources_Index, len(indexYAML.Indexes))
		for i, idx := range indexYAML.Indexes {
			entry := deploy.AppEngineResources_Index{
				Kind:     idx.Kind,
				Ancestor: yesOrBoolToBool(idx.Ancestor),
				Property: make([]*deploy.AppEngineResources_Index_Property, len(idx.Properties)),
			}
			for pidx, idxProp := range idx.Properties {
				dir, err := deploy.IndexDirectionFromAppYAMLString(idxProp.Direction)
				if err != nil {
					return nil, errors.Annotate(err, "could not identify direction for entry").
						InternalReason("index(%d)/propertyIndex(%d)", i, pidx).Err()
				}

				entry.Property[pidx] = &deploy.AppEngineResources_Index_Property{
					Name:      idxProp.Name,
					Direction: dir,
				}
			}

			res.Index[i] = &entry
		}
	}
	return &res, nil
}

func intPtrOrNilIfZero(v uint32) *int {
	if v == 0 {
		return nil
	}

	value := int(v)
	return &value
}

func yesOrBoolToBool(v interface{}) bool {
	switch t := v.(type) {
	case string:
		return (t == "yes")
	case bool:
		return t
	default:
		return false
	}
}
