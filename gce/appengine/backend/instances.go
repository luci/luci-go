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

package backend

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/google/uuid"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/gce/api/tasks/v1"
	"go.chromium.org/luci/gce/appengine/backend/internal/metrics"
	"go.chromium.org/luci/gce/appengine/model"
)

var (
	// Check createVM in queues.go for the naming logic
	hostnameSuffixMatch = regexp.MustCompile(`-[0-9]+-[a-z0-9]{4}$`)
)

// setCreated sets the GCE instance as created in the datastore if it isn't already.
func setCreated(c context.Context, id string, inst *compute.Instance) error {
	t, err := time.Parse(time.RFC3339, inst.CreationTimestamp)
	if err != nil {
		return errors.Fmt("failed to parse instance creation time: %w", err)
	}
	nics := make([]model.NetworkInterface, len(inst.NetworkInterfaces))
	for i, n := range inst.NetworkInterfaces {
		if len(n.AccessConfigs) > 0 {
			// GCE currently supports at most one access config per network interface.
			nics[i].ExternalIP = n.AccessConfigs[0].NatIP
			if len(n.AccessConfigs) > 1 {
				logging.Warningf(c, "network interface %q has more than one access config", n.Name)
			}
		}
		nics[i].InternalIP = n.NetworkIP
	}
	vm := &model.VM{
		ID: id,
	}
	put := false
	err = datastore.RunInTransaction(c, func(c context.Context) error {
		put = false
		switch err := datastore.Get(c, vm); {
		case err != nil:
			return errors.Fmt("failed to fetch VM with id: %q: %w", id, err)
		case vm.Created > 0:
			return nil
		}
		vm.Created = t.Unix()
		vm.NetworkInterfaces = nics
		vm.URL = inst.SelfLink
		if err := datastore.Put(c, vm); err != nil {
			return errors.Fmt("failed to store VM %q: %w", id, err)
		}
		put = true
		return nil
	}, nil)
	if put && err == nil {
		metrics.ReportCreationTime(c, float64(vm.Created-vm.ConfigExpanded), vm.Prefix, vm.Attributes.GetProject(), vm.Attributes.GetZone())
	}
	return err
}

// logErrors logs the errors in the given *googleapi.Error.
func logErrors(c context.Context, actionSource, hostname string, err *googleapi.Error) {
	var errMsgs []string
	for _, err := range err.Errors {
		errMsgs = append(errMsgs, err.Message)
	}
	logging.Errorf(c, "%s %q: failed with HTTP: %d, errors: %s", actionSource, hostname, err.Code, strings.Join(errMsgs, ","))
}

// rateLimitExceeded returns whether the given *googleapi.Error contains a rate
// limit error.
func rateLimitExceeded(err *googleapi.Error) bool {
	for _, err := range err.Errors {
		switch {
		case strings.Contains(err.Message, "Queries per user per 100 seconds"):
			return true
		case strings.Contains(err.Message, "Rate Limit Exceeded"):
			return true
		case strings.Contains(err.Message, "rateLimitExceeded"):
			return true
		case strings.Contains(err.Reason, "rateLimitExceeded"):
			return true
		}
	}
	return false
}

// checkInstance fetches the GCE instance and either sets its creation details
// or deletes the VM if the instance doesn't exist.
func checkInstance(c context.Context, vm *model.VM) error {
	logging.Debugf(c, "Check created instance %q: checking instance creation", vm.Hostname)
	srv := getCompute(c).Stable.Instances
	call := srv.Get(vm.Attributes.GetProject(), vm.Attributes.GetZone(), vm.Hostname)
	inst, err := call.Context(c).Do()
	if err != nil {
		if gerr, ok := err.(*googleapi.Error); ok {
			if gerr.Code == http.StatusNotFound {
				logging.Debugf(c, "Check created instance %q: instance not found in %q project", vm.Hostname, vm.Attributes.GetProject())
				metrics.UpdateFailures(c, gerr.Code, vm)
				if err := deleteVM(c, vm.ID, vm.Hostname); err != nil {
					return errors.Fmt("check created instance %q: not found: %w", vm.Hostname, err)
				}
				return errors.Fmt("create instance %q: not found: %w", vm.Hostname, err)
			}
			logErrors(c, "Check created instance", vm.Hostname, gerr)
		}
		logging.Debugf(c, "Check created instance %q: fail to find in %q project", vm.Hostname, vm.Attributes.GetProject())
		return errors.Fmt("failed to fetch instance: %w", err)
	}
	logging.Debugf(c, "Check created instance %q: successfully created %s", vm.Hostname, inst.SelfLink)
	metrics.CreatedInstanceChecked.Add(c, 1, vm.Config, vm.Attributes.GetProject(), vm.ScalingType, vm.Attributes.GetZone(), vm.Hostname)
	return setCreated(c, vm.ID, inst)
}

// createInstanceQueue is the name of the create instance task handler queue.
const createInstanceQueue = "create-instance"

// createInstance creates a GCE instance.
func createInstance(ctx context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.CreateInstance)
	switch {
	case !ok:
		return errors.Fmt("create instance: unexpected payload %q", payload)
	case task.GetId() == "":
		return errors.New("create instance: instance ID is required")
	}
	vm := &model.VM{
		ID: task.Id,
	}
	switch err := datastore.Get(ctx, vm); {
	case err != nil:
		return errors.Fmt("Create instance: failed to fetch VM with id: %q: %w", task.GetId(), err)
	case vm.URL != "":
		logging.Debugf(ctx, "Create instance: instance already %s", vm.URL)
		return nil
	}
	logging.Debugf(ctx, "Create instance %q: with ID %q", vm.Hostname, vm.ID)
	instance := vm.GetInstance()

	// Generate a request ID based on the hostname.
	// Ensures duplicate operations aren't created in GCE.
	// Request IDs are valid for 24 hours.
	rID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("create-%s", vm.Hostname)))
	srv := getCompute(ctx)
	op, err := srv.InsertInstance(ctx, vm.Attributes.GetProject(), vm.Attributes.GetZone(), instance, rID.String())
	if err != nil {
		logging.Debugf(ctx, "Create instance %q: got error from attempt to create instance %s", vm.Hostname, err)
		if gerr, ok := err.(*googleapi.Error); ok {
			logErrors(ctx, "Create instance", vm.Hostname, gerr)
			metrics.UpdateFailures(ctx, gerr.Code, vm)
			// TODO(b/130826296): Remove this once rate limit returns a transient HTTP error code.
			if rateLimitExceeded(gerr) {
				return errors.Fmt("rate limit exceeded creating instance %s: %w", vm.Hostname, err)
			}
			if gerr.Code == http.StatusTooManyRequests || gerr.Code >= 500 {
				return errors.Fmt("transiently failed to create instance %s: %w", vm.Hostname, err)
			}
			logging.Debugf(ctx, "Create instance %q: try to delete instance as got error during creation.", vm.Hostname)
			if err := deleteVM(ctx, task.Id, vm.Hostname); err != nil {
				logging.Errorf(ctx, "Create instance %q: failed to delete instance %s", vm.Hostname, err)
			}
		}
		return errors.Fmt("failed to create instance %s: %w", vm.Hostname, err)
	}
	logging.Debugf(ctx, "Create instance %q: received response from GCP, waiting execution", vm.Hostname)
	if operationsErrors := op.GetErrors(); len(operationsErrors) > 0 {
		logging.Debugf(ctx, "Create instance %q: failed to create instance total %d error received", vm.Hostname, len(operationsErrors))
		for _, err := range operationsErrors {
			logging.Errorf(ctx, "create instance %q: failed with code %s: Message %s", vm.Hostname, err.Code, err.Message)
		}
		metrics.UpdateFailures(ctx, 200, vm)
		if err := deleteVM(ctx, task.Id, vm.Hostname); err != nil {
			return errors.Fmt("failed to create instance %s: %w", vm.Hostname, err)
		}
		return errors.Fmt("failed to create instance %s", vm.Hostname)
	}
	if op.GetStatus() == "DONE" {
		logging.Debugf(ctx, "Create instance %q: reported as created, next step is to check it.", vm.Hostname)
		return checkInstance(ctx, vm)
	}
	// Instance creation is pending.
	logging.Debugf(ctx, "Create instance %q: instance not created yet, waiting, will check with next attempt", vm.Hostname)
	return nil
}

// destroyInstanceAsync schedules a task queue task to destroy a GCE instance.
func destroyInstanceAsync(c context.Context, id, url string) error {
	t := &tq.Task{
		Payload: &tasks.DestroyInstance{
			Id:  id,
			Url: url,
		},
	}
	if err := getDispatcher(c).AddTask(c, t); err != nil {
		return errors.Fmt("failed to schedule destroy task: %w", err)
	}
	return nil
}

// destroyInstanceQueue is the name of the destroy instance task handler queue.
const destroyInstanceQueue = "destroy-instance"

// destroyInstance destroys a GCE instance.
func destroyInstance(c context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.DestroyInstance)
	switch {
	case !ok:
		return errors.Fmt("unexpected payload type %T", payload)
	case task.GetId() == "":
		return errors.New("ID is required")
	case task.GetUrl() == "":
		return errors.New("URL is required")
	}
	vm := &model.VM{
		ID: task.Id,
	}
	switch err := datastore.Get(c, vm); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil
	case err != nil:
		return errors.Fmt("destroy instance: failed to fetch VM %s: %w", vm.ID, err)
	case vm.URL != task.Url:
		// Instance is already destroyed and replaced. Don't destroy the new one.
		logging.Debugf(c, "Destroy instance %q: does not exist %s", vm.Hostname, task.Url)
		return nil
	}
	logging.Debugf(c, "Destroy instance %q: starting...", vm.Hostname)
	rID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("destroy-%s", vm.Hostname)))
	srv := getCompute(c).Stable.Instances
	call := srv.Delete(vm.Attributes.GetProject(), vm.Attributes.GetZone(), vm.Hostname)
	op, err := call.RequestId(rID.String()).Context(c).Do()
	metrics.DestroyInstanceUnchecked.Add(c, 1, vm.Config, vm.Attributes.GetProject(), vm.Attributes.GetZone(), vm.Hostname)
	if err != nil {
		if gerr, ok := err.(*googleapi.Error); ok {
			if gerr.Code == http.StatusNotFound {
				// Instance is already destroyed.
				logging.Debugf(c, "Distroy instance %q: does not exist: %s", vm.Hostname, vm.URL)
				return deleteBotAsync(c, task.Id, vm.Hostname)
			}
			logErrors(c, "Destroy instance", vm.Hostname, gerr)
		}
		logging.Debugf(c, "Destroy instance %q: failed to destroy %s", vm.Hostname, err)
		return errors.Fmt("destroy instance %q: %w", vm.Hostname, err)
	}
	if op.Error != nil && len(op.Error.Errors) > 0 {
		logging.Debugf(c, "Destroy instance %q: failed to destroy with %d erroros: %s", vm.Hostname, len(op.Error.Errors))
		for _, err := range op.Error.Errors {
			logging.Errorf(c, "Destroy instance %q: failed with code %s: %s", vm.Hostname, err.Code, err.Message)
		}
		return errors.Fmt("Destroy instance %q: failed to destroy", vm.Hostname)
	}
	if op.Status == "DONE" {
		logging.Debugf(c, "Destroy instance %q: done", op.TargetLink)
		return deleteBotAsync(c, task.Id, vm.Hostname)
	}
	logging.Debugf(c, "Destroy instance %q: instance not destroyed yet, waiting, will check with next attempt.", vm.Hostname)
	// Instance destruction is pending.
	return nil
}

const auditInstancesQueue = "audit-instances"

// Number of instances to audit per run
const auditBatchSize = 100

// auditInstanceInZone attempts to enforce the vm table as the source of truth
// for any given project. It does this by deleting anything that it doesn't
// recognize.
func auditInstanceInZone(c context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.AuditProject)
	switch {
	case !ok:
		return errors.Fmt("Unexpected payload type %T", payload)
	case task.GetProject() == "":
		return errors.New("Project is required")
	case task.GetZone() == "":
		return errors.New("Zone is required")
	}
	proj := task.GetProject()
	zone := task.GetZone()
	logging.Debugf(c, "Audit instances in project %s, zone %s", proj, zone)
	// List a bunch on instances and validate with the DB
	srv := getCompute(c).Stable.Instances
	call := srv.List(proj, zone).MaxResults(auditBatchSize)
	if task.GetPageToken() != "" {
		// Add the page token if given
		call = call.PageToken(task.GetPageToken())
	}
	op, err := call.Context(c).Do()
	if err != nil {
		if gerr, ok := err.(*googleapi.Error); ok {
			logErrors(c, "Audit instance", fmt.Sprintf("%s-%s", proj, zone), gerr)
		}
		return errors.Fmt("failed to list  %s-%s: %w", proj, zone, err)
	}

	if op.Warning != nil {
		logging.Warningf(c, "Audit instance %q: %s: %s", proj, op.Warning.Code, op.Warning.Message)
	}

	// Assign job to handle the next set of instances
	defer func(c context.Context, task *tasks.AuditProject, pageToken string) {
		if pageToken != "" {
			task.PageToken = pageToken
			if err := getDispatcher(c).AddTask(c, &tq.Task{
				Payload: task,
			}); err != nil {
				logging.Errorf(c, "Audit instance %s: failed to add audit task for %s-%s", proj, proj, zone)
			}
		}
	}(c, task, op.NextPageToken)

	// Mapping from hostname to VM
	hostToVM := make(map[string]*model.VM)
	for _, inst := range op.Items {
		// Init all the hostnames. Or as gcloud calls it names. There
		// is no hostname assigned to this VM in gclouds sense. This is
		// because it has a different meaning. We mean the hostname
		// from swarmings perspective, which is gclouds sense is just
		// name. The instance struct does have a hostname field, but
		// this is empty for all the VMs
		hostToVM[inst.Name] = nil
	}

	query := datastore.NewQuery(model.VMKind)
	// Queries will contain all the queries to be made to datastore
	var queries []*datastore.Query
	for hostname := range hostToVM {
		// Eq creates a new object everytime. So collect all the unique
		// queries
		queries = append(queries, query.Eq("hostname", hostname))
	}
	mapVMs := func(vm *model.VM) {
		hostToVM[vm.Hostname] = vm
	}
	// Get all the VM records corresponding to the listed instances
	err = datastore.RunMulti(c, queries, mapVMs)
	if err != nil {
		logging.Errorf(c, "Audit instance: failed to query for the VMS. %v", err)
		return err
	}
	var countLeaks int64
	// Delete the ones we don't know of
	for hostname, vm := range hostToVM {
		if vm == nil && isLeakHuerestic(c, hostname, proj, zone) {
			countLeaks += 1
			logging.Debugf(c, "Audit instance: plugging the instance leak in %s-%s: %s", proj, zone, hostname)
			// Send a delete request for the instance
			reqID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("plug-%s", hostname)))
			del := srv.Delete(proj, zone, hostname)
			op, err := del.RequestId(reqID.String()).Context(c).Do()
			if err != nil {
				if gerr, ok := err.(*googleapi.Error); ok {
					if gerr.Code == http.StatusNotFound {
						// Instance is already destroyed.
						logging.Debugf(c, "instance does not exist: %s", hostname)
					}
					logErrors(c, "Audit instance", hostname, gerr)
				}
				logging.Errorf(c, "Audit instance: failed to plug the leak %s. %v", hostname, err)
				continue
			}
			if op.Error != nil && len(op.Error.Errors) > 0 {
				for _, e := range op.Error.Errors {
					logging.Errorf(c, "%s: %s", e.Code, e.Message)
				}
				logging.Errorf(c, "Audit instance: failed to plug the leak %s. %v", hostname, err)
				continue
			}
			if op.Status == "DONE" {
				logging.Debugf(c, "Audit instance: plugged the leak of instance: %s", op.TargetLink)
			}
		}
	}
	logging.Debugf(c, "Audit instance %s: finished!", proj)
	metrics.UpdateLeaks(c, countLeaks, proj, zone)
	return nil
}

// isLeakHuerestic determines if the given hostname is a leak from gce-provider
// using a heuristic.
func isLeakHuerestic(ctx context.Context, hostname, proj, zone string) bool {
	// If this was an instance created by gce-provider. There must be a set
	// of VM records for the prefix matching the hostname. Checking the
	// existence of these will let us determine if this was a leak
	id := hostname[:len(hostname)-5] // Remove the last 5 characters
	prefix := hostnameSuffixMatch.ReplaceAllString(hostname, "")
	q := datastore.NewQuery(model.VMKind)
	// Get all the VMs under the prefix
	q = q.Eq("prefix", prefix)
	// Update the leak var based on the VM records that we find
	leak := false
	cb := func(vm *model.VM) error {
		switch {
		case vm.Hostname == hostname:
			logging.Debugf(ctx, "%s record exists. Not a leak", hostname)
			leak = false
			// Stop searing as this is not a leak
			return datastore.Stop
		case vm.ID == id:
			logging.Debugf(ctx, "%s is a leaked instance replaced by %s", hostname, vm.Hostname)
			leak = true
			// Stop searching as this is a leak
			return datastore.Stop
		default:
			// Prefix matches but we cannot find the id that matches. Maybe the pool
			// was resized. Might be a leak unless above conditions are true for
			// other matches
			leak = true
		}
		return nil
	}
	err := datastore.Run(ctx, q, cb)
	if err != nil {
		logging.Errorf(ctx, "isLeakHuerestic -- [%s]Error querying DB %v", hostname, err)
		leak = false
	}
	if leak {
		logging.Debugf(ctx, "%s is probably a leak in %s and %s", hostname, proj, zone)
	} else {
		logging.Debugf(ctx, "Cannot determine if %s is a leak in %s and %s", hostname, proj, zone)
	}
	return leak
}
