// Copyright 2026 The LUCI Authors.
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

import {
  ProductCatalogEntry,
  GceProductCatalogEntry,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export interface UnifiedProductCatalogEntry {
  readonly productCatalogId: string;
  readonly productName: string;
  readonly descriptiveName: string;
  readonly fleetPlmStatus?: string;
  readonly productType: string;

  // Standard product catalog fields
  readonly gpn?: string;
  readonly resourceType?: string;
  readonly r11n?: readonly string[];
  readonly numberOfDevicesPerRack?: number;
  readonly unitCost?: string;

  // GCE specific fields
  readonly cpuType?: string;
  readonly cpuNumPerVm?: number;
  readonly memoryGbPerVm?: number;
}

export type CatalogColumnKey = keyof UnifiedProductCatalogEntry;

export function fromStandardCatalogEntry(
  entry: ProductCatalogEntry,
): UnifiedProductCatalogEntry {
  return {
    productCatalogId: entry.productCatalogId,
    productName: entry.productName,
    gpn: entry.gpn,
    descriptiveName: entry.descriptiveName,
    resourceType: entry.resourceType,
    fleetPlmStatus: entry.fleetPlmStatus,
    r11n: entry.r11n,
    numberOfDevicesPerRack: entry.numberOfDevicesPerRack,
    unitCost: entry.unitCost,
    productType: entry.productType,
  };
}

export function fromGceCatalogEntry(
  entry: GceProductCatalogEntry,
): UnifiedProductCatalogEntry {
  return {
    productCatalogId: entry.productCatalogId,
    productName: entry.productName,
    descriptiveName: entry.descriptiveName,
    fleetPlmStatus: entry.fleetPlmStatus,
    productType: 'gce',
    cpuType: entry.cpuType,
    cpuNumPerVm: entry.cpuNumPerVm,
    memoryGbPerVm: entry.memoryGbPerVm,
  };
}
