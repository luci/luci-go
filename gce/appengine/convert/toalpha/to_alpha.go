// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// toalpha contains functions that convert stable cloud protos to alpha cloud protos.
package toalpha

import (
	computealpha "google.golang.org/api/compute/v0.alpha"
	compute "google.golang.org/api/compute/v1"
)

// MapSlice takes a slice of Ts, applies a function to each T, and returns a slice of Us.
func MapSlice[T any, U any](input []T, f func(T) U) []U {
	// We want a zero struct to have nil, not []U{} as the value for
	// its repeated fields.
	if len(input) == 0 {
		return nil
	}
	out := make([]U, len(input))
	for i, k := range input {
		out[i] = f(k)
	}
	return out
}


// Keep this functions in alphabetical order. They should be named after the type that they convert from stable to alpha. Thank you.

// AcceleratorConfig converts a stable AcceleratorConfig to an alpha AcceleratorConfig.
func AcceleratorConfig(config *compute.AcceleratorConfig) *computealpha.AcceleratorConfig {
	if config == nil {
		return nil
	}
	return &computealpha.AcceleratorConfig{
		AcceleratorCount: config.AcceleratorCount,
		AcceleratorType:  config.AcceleratorType,
		ForceSendFields:  config.ForceSendFields,
		NullFields:       config.NullFields,
	}
}

// AccessConfig converts a stable AccessConfig to an alpha AccessConfig.
func AccessConfig(config *compute.AccessConfig) *computealpha.AccessConfig {
	if config == nil {
		return nil
	}
	return &computealpha.AccessConfig{
		ExternalIpv6:             config.ExternalIpv6,
		ExternalIpv6PrefixLength: config.ExternalIpv6PrefixLength,
		Kind:                     config.Kind,
		Name:                     config.Name,
		NatIP:                    config.NatIP,
		NetworkTier:              config.NetworkTier,
		PublicPtrDomainName:      config.PublicPtrDomainName,
		SetPublicPtr:             config.SetPublicPtr,
		Type:                     config.Type,
		ForceSendFields:          config.ForceSendFields,
		NullFields:               config.NullFields,
	}
}

// AdvancedMachineFeatures converts a stable AdvancedMachineFeatures to an alpha AdvancedMachineFeatures.
func AdvancedMachineFeatures(features *compute.AdvancedMachineFeatures) *computealpha.AdvancedMachineFeatures {
	if features == nil {
		return nil
	}
	return &computealpha.AdvancedMachineFeatures{
		EnableNestedVirtualization: features.EnableNestedVirtualization,
		EnableUefiNetworking:       features.EnableUefiNetworking,
		ThreadsPerCore:             features.ThreadsPerCore,
		VisibleCoreCount:           features.VisibleCoreCount,
		ForceSendFields:            features.ForceSendFields,
		NullFields:                 features.NullFields,
	}
}

// AliasIpRange converts a stable AliasIpRange to an alpha AliasIpRange.
func AliasIpRange(rng *compute.AliasIpRange) *computealpha.AliasIpRange {
	if rng == nil {
		return nil
	}
	return &computealpha.AliasIpRange{
		IpCidrRange:         rng.IpCidrRange,
		SubnetworkRangeName: rng.SubnetworkRangeName,
		ForceSendFields:     rng.ForceSendFields,
		NullFields:          rng.NullFields,
	}
}

// AttachedDiskInitializeParams converts a stable AttachedDiskInitializeParams to an alpha AttachedDiskInitializeParams.
func AttachedDiskInitializeParams(params *compute.AttachedDiskInitializeParams) *computealpha.AttachedDiskInitializeParams {
	if params == nil {
		return nil
	}
	return &computealpha.AttachedDiskInitializeParams{
		Architecture:                params.Architecture,
		Description:                 params.Description,
		DiskName:                    params.DiskName,
		DiskSizeGb:                  params.DiskSizeGb,
		DiskType:                    params.DiskType,
		Labels:                      params.Labels,
		Licenses:                    params.Licenses,
		OnUpdateAction:              params.OnUpdateAction,
		ProvisionedIops:             params.ProvisionedIops,
		ProvisionedThroughput:       params.ProvisionedThroughput,
		ReplicaZones:                params.ReplicaZones,
		ResourceManagerTags:         params.ResourceManagerTags,
		ResourcePolicies:            params.ResourcePolicies,
		SourceImage:                 params.SourceImage,
		SourceImageEncryptionKey:    CustomerEncryptionKey(params.SourceImageEncryptionKey),
		SourceSnapshot:              params.SourceSnapshot,
		SourceSnapshotEncryptionKey: CustomerEncryptionKey(params.SourceSnapshotEncryptionKey),
		ForceSendFields:             params.ForceSendFields,
		NullFields:                  params.NullFields,
	}
}

// AttachedDisk converts a stable AttachedDisk to an alpha AttachedDisk.
func AttachedDisk(disk *compute.AttachedDisk) *computealpha.AttachedDisk {
	if disk == nil {
		return nil
	}
	return &computealpha.AttachedDisk{
		Architecture:                 disk.Architecture,
		AutoDelete:                   disk.AutoDelete,
		Boot:                         disk.Boot,
		DeviceName:                   disk.DeviceName,
		DiskEncryptionKey:            CustomerEncryptionKey(disk.DiskEncryptionKey),
		DiskSizeGb:                   disk.DiskSizeGb,
		ForceAttach:                  disk.ForceAttach,
		GuestOsFeatures:              MapSlice(disk.GuestOsFeatures, GuestOsFeature),
		Index:                        disk.Index,
		InitializeParams:             AttachedDiskInitializeParams(disk.InitializeParams),
		Interface:                    disk.Interface,
		Kind:                         disk.Kind,
		Licenses:                     disk.Licenses,
		Mode:                         disk.Mode,
		SavedState:                   disk.SavedState,
		ShieldedInstanceInitialState: InitialStateConfig(disk.ShieldedInstanceInitialState),
		Source:                       disk.Source,
		Type:                         disk.Type,
		ForceSendFields:              disk.ForceSendFields,
		NullFields:                   disk.NullFields,
	}
}

// CustomerEncryptionKey converts a stable CustomerEncryptionKey to an alpha CustomerEncryptionKey.
func CustomerEncryptionKey(key *compute.CustomerEncryptionKey) *computealpha.CustomerEncryptionKey {
	if key == nil {
		return nil
	}
	return &computealpha.CustomerEncryptionKey{
		KmsKeyName:           key.KmsKeyName,
		KmsKeyServiceAccount: key.KmsKeyServiceAccount,
		RawKey:               key.RawKey,
		RsaEncryptedKey:      key.RsaEncryptedKey,
		Sha256:               key.Sha256,
		ForceSendFields:      key.ForceSendFields,
		NullFields:           key.NullFields,
	}
}

// DisplayDevice converts a stable DisplayDevice to an alpha DisplayDevice.
func DisplayDevice(device *compute.DisplayDevice) *computealpha.DisplayDevice {
	if device == nil {
		return nil
	}
	return &computealpha.DisplayDevice{
		EnableDisplay:   device.EnableDisplay,
		ForceSendFields: device.ForceSendFields,
		NullFields:      device.NullFields,
	}
}

// Duration converts a stable Duration to an alpha Duration.
func Duration(duration *compute.Duration) *computealpha.Duration {
	if duration == nil {
		return nil
	}
	return &computealpha.Duration{
		Nanos:           duration.Nanos,
		Seconds:         duration.Seconds,
		ForceSendFields: duration.ForceSendFields,
		NullFields:      duration.NullFields,
	}
}

// FileContentBuffer converts a stable FileContentBuffer to an alpha FileContentBuffer.
func FileContentBuffer(buffer *compute.FileContentBuffer) *computealpha.FileContentBuffer {
	if buffer == nil {
		return nil
	}
	return &computealpha.FileContentBuffer{
		Content:         buffer.Content,
		FileType:        buffer.FileType,
		ForceSendFields: buffer.ForceSendFields,
		NullFields:      buffer.NullFields,
	}
}

// GuestOsFeature converts a stable GuestOsFeature to an alpha GuestOsFeature.
func GuestOsFeature(feature *compute.GuestOsFeature) *computealpha.GuestOsFeature {
	if feature == nil {
		return nil
	}
	return &computealpha.GuestOsFeature{
		Type:            feature.Type,
		ForceSendFields: feature.ForceSendFields,
		NullFields:      feature.NullFields,
	}
}

// InstanceParams converts a stable InstanceParams to an alpha InstanceParams.
func InstanceParams(params *compute.InstanceParams) *computealpha.InstanceParams {
	if params == nil {
		return nil
	}
	return &computealpha.InstanceParams{
		ResourceManagerTags: params.ResourceManagerTags,
		ForceSendFields:     params.ForceSendFields,
		NullFields:          params.NullFields,
	}
}

// InitialStateConfig converts a stable InitialStateConfig to an alpha InitialStateConfig.
func InitialStateConfig(config *compute.InitialStateConfig) *computealpha.InitialStateConfig {
	if config == nil {
		return nil
	}
	return &computealpha.InitialStateConfig{
		Dbs:             MapSlice(config.Dbs, FileContentBuffer),
		Dbxs:            MapSlice(config.Dbxs, FileContentBuffer),
		Keks:            MapSlice(config.Keks, FileContentBuffer),
		Pk:              FileContentBuffer(config.Pk),
		ForceSendFields: config.ForceSendFields,
		NullFields:      config.NullFields,
	}
}

// Instance converts a stable Instance to an alpha Instance.
func Instance(instance *compute.Instance) *computealpha.Instance {
	if instance == nil {
		return nil
	}
	return &computealpha.Instance{
		AdvancedMachineFeatures:         AdvancedMachineFeatures(instance.AdvancedMachineFeatures),
		CanIpForward:                    instance.CanIpForward,
		ConfidentialInstanceConfig:      InstanceConfig(instance.ConfidentialInstanceConfig),
		CpuPlatform:                     instance.CpuPlatform,
		CreationTimestamp:               instance.CreationTimestamp,
		DeletionProtection:              instance.DeletionProtection,
		Description:                     instance.Description,
		Disks:                           MapSlice(instance.Disks, AttachedDisk),
		DisplayDevice:                   DisplayDevice(instance.DisplayDevice),
		Fingerprint:                     instance.Fingerprint,
		GuestAccelerators:               MapSlice(instance.GuestAccelerators, AcceleratorConfig),
		Hostname:                        instance.Hostname,
		Id:                              instance.Id,
		InstanceEncryptionKey:           CustomerEncryptionKey(instance.InstanceEncryptionKey),
		KeyRevocationActionType:         instance.KeyRevocationActionType,
		Kind:                            instance.Kind,
		LabelFingerprint:                instance.LabelFingerprint,
		Labels:                          instance.Labels,
		LastStartTimestamp:              instance.LastStartTimestamp,
		LastStopTimestamp:               instance.LastStopTimestamp,
		LastSuspendedTimestamp:          instance.LastSuspendedTimestamp,
		MachineType:                     instance.MachineType,
		Metadata:                        Metadata(instance.Metadata),
		MinCpuPlatform:                  instance.MinCpuPlatform,
		Name:                            instance.Name,
		NetworkInterfaces:               MapSlice(instance.NetworkInterfaces, NetworkInterface),
		NetworkPerformanceConfig:        NetworkPerformanceConfig(instance.NetworkPerformanceConfig),
		Params:                          InstanceParams(instance.Params),
		PrivateIpv6GoogleAccess:         instance.PrivateIpv6GoogleAccess,
		ReservationAffinity:             ReservationAffinity(instance.ReservationAffinity),
		ResourcePolicies:                instance.ResourcePolicies,
		ResourceStatus:                  ResourceStatus(instance.ResourceStatus),
		SatisfiesPzs:                    instance.SatisfiesPzs,
		Scheduling:                      Scheduling(instance.Scheduling),
		SelfLink:                        instance.SelfLink,
		ServiceAccounts:                 MapSlice(instance.ServiceAccounts, ServiceAccount),
		ShieldedInstanceConfig:          ShieldedInstanceConfig(instance.ShieldedInstanceConfig),
		ShieldedInstanceIntegrityPolicy: ShieldedInstanceIntegrityPolicy(instance.ShieldedInstanceIntegrityPolicy),
		SourceMachineImage:              instance.SourceMachineImage,
		SourceMachineImageEncryptionKey: CustomerEncryptionKey(instance.SourceMachineImageEncryptionKey),
		StartRestricted:                 instance.StartRestricted,
		Status:                          instance.Status,
		StatusMessage:                   instance.StatusMessage,
		Tags:                            Tags(instance.Tags),
		Zone:                            instance.Zone,
		ServerResponse:                  instance.ServerResponse,
		ForceSendFields:                 instance.ForceSendFields,
		NullFields:                      instance.NullFields,
	}
}

// InstanceConfig converts a stable ConfidentialInstanceConfig to an alpha ConfidentialInstanceConfig.
func InstanceConfig(config *compute.ConfidentialInstanceConfig) *computealpha.ConfidentialInstanceConfig {
	if config == nil {
		return nil
	}
	return &computealpha.ConfidentialInstanceConfig{
		EnableConfidentialCompute: config.EnableConfidentialCompute,
		ForceSendFields:           config.ForceSendFields,
		NullFields:                config.NullFields,
	}
}

// Metadata converts a stable Metadata to an alpha Metadata.
func Metadata(metadata *compute.Metadata) *computealpha.Metadata {
	if metadata == nil {
		return nil
	}
	return &computealpha.Metadata{
		Fingerprint:     metadata.Fingerprint,
		Items:           MapSlice(metadata.Items, MetadataItems),
		Kind:            metadata.Kind,
		ForceSendFields: metadata.ForceSendFields,
		NullFields:      metadata.NullFields,
	}
}

// MetadataItems converts a stable MetadataItems to an alpha MetadataItems.
func MetadataItems(items *compute.MetadataItems) *computealpha.MetadataItems {
	if items == nil {
		return nil
	}
	return &computealpha.MetadataItems{
		Key:             items.Key,
		Value:           items.Value,
		ForceSendFields: items.ForceSendFields,
		NullFields:      items.NullFields,
	}
}

// NetworkInterface converts a stable NetworkInterface to an alpha NetworkInterface.
func NetworkInterface(intf *compute.NetworkInterface) *computealpha.NetworkInterface {
	if intf == nil {
		return nil
	}
	return &computealpha.NetworkInterface{
		AccessConfigs:            MapSlice(intf.AccessConfigs, AccessConfig),
		AliasIpRanges:            MapSlice(intf.AliasIpRanges, AliasIpRange),
		Fingerprint:              intf.Fingerprint,
		InternalIpv6PrefixLength: intf.InternalIpv6PrefixLength,
		Ipv6AccessConfigs:        MapSlice(intf.Ipv6AccessConfigs, AccessConfig),
		Ipv6AccessType:           intf.Ipv6AccessType,
		Ipv6Address:              intf.Ipv6Address,
		Kind:                     intf.Kind,
		Name:                     intf.Name,
		Network:                  intf.Network,
		NetworkAttachment:        intf.NetworkAttachment,
		NetworkIP:                intf.NetworkIP,
		NicType:                  intf.NicType,
		QueueCount:               intf.QueueCount,
		StackType:                intf.StackType,
		Subnetwork:               intf.Subnetwork,
		ForceSendFields:          intf.ForceSendFields,
		NullFields:               intf.NullFields,
	}
}

// NetworkPerformanceConfig converts a stable NetworkPerformanceConfig to an alpha NetworkPerformanceConfig.
func NetworkPerformanceConfig(config *compute.NetworkPerformanceConfig) *computealpha.NetworkPerformanceConfig {
	if config == nil {
		return nil
	}
	return &computealpha.NetworkPerformanceConfig{
		TotalEgressBandwidthTier: config.TotalEgressBandwidthTier,
		ForceSendFields:          config.ForceSendFields,
		NullFields:               config.NullFields,
	}
}

// ReservationAffinity converts a stable ReservationAffinity to an alpha ReservationAffinity.
func ReservationAffinity(affinity *compute.ReservationAffinity) *computealpha.ReservationAffinity {
	if affinity == nil {
		return nil
	}
	return &computealpha.ReservationAffinity{
		ConsumeReservationType: affinity.ConsumeReservationType,
		Key:                    affinity.Key,
		Values:                 affinity.Values,
		ForceSendFields:        affinity.ForceSendFields,
		NullFields:             affinity.NullFields,
	}
}

// ResourceStatus converts a stable ResourceStatus to an alpha ResourceStatus.
func ResourceStatus(status *compute.ResourceStatus) *computealpha.ResourceStatus {
	if status == nil {
		return nil
	}
	return &computealpha.ResourceStatus{
		PhysicalHost:    status.PhysicalHost,
		ForceSendFields: status.ForceSendFields,
		NullFields:      status.NullFields,
	}
}

// Scheduling converts a stable Scheduling to an alpha Scheduling.
func Scheduling(scheduling *compute.Scheduling) *computealpha.Scheduling {
	if scheduling == nil {
		return nil
	}
	return &computealpha.Scheduling{
		AutomaticRestart:          scheduling.AutomaticRestart,
		InstanceTerminationAction: scheduling.InstanceTerminationAction,
		LocalSsdRecoveryTimeout:   Duration(scheduling.LocalSsdRecoveryTimeout),
		LocationHint:              scheduling.LocationHint,
		MinNodeCpus:               scheduling.MinNodeCpus,
		NodeAffinities:            MapSlice(scheduling.NodeAffinities, SchedulingNodeAffinity),
		OnHostMaintenance:         scheduling.OnHostMaintenance,
		Preemptible:               scheduling.Preemptible,
		ProvisioningModel:         scheduling.ProvisioningModel,
		ForceSendFields:           scheduling.ForceSendFields,
		NullFields:                scheduling.NullFields,
	}
}

// SchedulingNodeAffinity converts a stable SchedulingNodeAffinity to an alpha SchedulingNodeAffinity.
func SchedulingNodeAffinity(affinity *compute.SchedulingNodeAffinity) *computealpha.SchedulingNodeAffinity {
	if affinity == nil {
		return nil
	}
	return &computealpha.SchedulingNodeAffinity{
		Key:             affinity.Key,
		Operator:        affinity.Operator,
		Values:          affinity.Values,
		ForceSendFields: affinity.ForceSendFields,
		NullFields:      affinity.NullFields,
	}
}

// ShieldedInstanceConfig converts a sstable ShieldedInstanceConfig to an alpha ShieldedInstanceConfig.
func ShieldedInstanceConfig(config *compute.ShieldedInstanceConfig) *computealpha.ShieldedInstanceConfig {
	if config == nil {
		return nil
	}
	return &computealpha.ShieldedInstanceConfig{
		EnableIntegrityMonitoring: config.EnableIntegrityMonitoring,
		EnableSecureBoot:          config.EnableSecureBoot,
		EnableVtpm:                config.EnableVtpm,
		ForceSendFields:           config.ForceSendFields,
		NullFields:                config.NullFields,
	}
}

// ShieldedInstanceIntegrityPolicy converts a stable ShieldedInstanceIntegrityPolicy to an alpha ShieldedInstanceIntegrityPolicy.
func ShieldedInstanceIntegrityPolicy(policy *compute.ShieldedInstanceIntegrityPolicy) *computealpha.ShieldedInstanceIntegrityPolicy {
	if policy == nil {
		return nil
	}
	return &computealpha.ShieldedInstanceIntegrityPolicy{
		UpdateAutoLearnPolicy: policy.UpdateAutoLearnPolicy,
		ForceSendFields:       policy.ForceSendFields,
		NullFields:            policy.NullFields,
	}
}

// ServiceAccount converts a stable ServiceAccount to an alpha ServiceAccount.
func ServiceAccount(account *compute.ServiceAccount) *computealpha.ServiceAccount {
	if account == nil {
		return nil
	}
	return &computealpha.ServiceAccount{
		Email:           account.Email,
		Scopes:          account.Scopes,
		ForceSendFields: account.ForceSendFields,
		NullFields:      account.NullFields,
	}
}

// Tags converts a stable Tags to an alpha Tags.
func Tags(tags *compute.Tags) *computealpha.Tags {
	if tags == nil {
		return nil
	}
	return &computealpha.Tags{
		Fingerprint:     tags.Fingerprint,
		Items:           tags.Items,
		ForceSendFields: tags.ForceSendFields,
		NullFields:      tags.NullFields,
	}
}
