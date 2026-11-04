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

import { StatusTone } from '@/fleet/theme/tokens';

import { UnifiedProductCatalogEntry } from './types';

export const GCE_ONLY_ACCESSORS = new Set<keyof UnifiedProductCatalogEntry>([
  'cpuType',
  'cpuNumPerVm',
  'memoryGbPerVm',
]);

export const HARDWARE_ONLY_ACCESSORS = new Set<
  keyof UnifiedProductCatalogEntry
>(['gpn', 'resourceType', 'r11n', 'numberOfDevicesPerRack', 'unitCost']);

export const getPlmStatusTone = (status?: string): StatusTone => {
  switch (status?.toUpperCase()) {
    case 'GA':
    case 'ACTIVE':
      return 'success';
    case 'LA':
      return 'warning';
    case 'NPI':
      return 'info';
    default:
      return 'neutral';
  }
};

export const resolveProductType = (
  productType?: string,
  productCatalogId?: string,
): string => {
  const explicit = (productType || '').trim().toLowerCase();
  if (explicit) return explicit;
  const upperId = (productCatalogId || '').trim().toUpperCase();
  if (upperId.startsWith('ATB-')) return 'android-testbed';
  if (upperId.startsWith('TB-')) return 'os-testbed';
  if (upperId.startsWith('PH-')) return 'hardware';
  if (upperId.startsWith('PP-')) return 'peripherals';
  if (upperId.startsWith('GC-') || upperId.startsWith('GCE-')) return 'gce';
  return '';
};

export const formatProductCategory = (
  productType?: string,
  productCatalogId?: string,
): string => {
  const prodType = resolveProductType(productType, productCatalogId);
  switch (prodType) {
    case 'android-testbed':
      return 'Android Testbed';
    case 'os-testbed':
      return 'OS Testbed';
    case 'hardware':
      return 'DUT & Host Hardware';
    case 'peripherals':
      return 'Lab Peripheral & Fixture';
    case 'gce':
      return 'GCE Virtual Machine';
    default:
      return productType || 'Catalog Item';
  }
};

export const formatUnitCost = (unitCost?: string): string => {
  if (!unitCost) return '—';
  const trimmed = unitCost.trim();
  if (!trimmed || trimmed === '0' || trimmed === '$0') return '—';
  const clean = trimmed.startsWith('$') ? trimmed.slice(1).trim() : trimmed;
  const numeric = Number(clean.replace(/,/g, ''));
  if (Number.isNaN(numeric)) {
    return trimmed;
  }
  if (numeric > 0) {
    return `$${numeric.toLocaleString('en-US')}`;
  }
  return '—';
};

export const copyTextToClipboard = async (text: string): Promise<boolean> => {
  if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // Fall through to execCommand if clipboard API rejects.
    }
  }
  if (typeof document !== 'undefined') {
    const textarea = document.createElement('textarea');
    textarea.value = text;
    textarea.style.position = 'fixed';
    textarea.style.opacity = '0';
    document.body.appendChild(textarea);
    try {
      textarea.select();
      return document.execCommand?.('copy') ?? false;
    } catch {
      return false;
    } finally {
      document.body.removeChild(textarea);
    }
  }
  return false;
};
