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

import { CatalogDiffItem } from './catalog_edit_prompt_dialog';
import { UnifiedProductCatalogEntry } from './types';

export const MIN_PHOTO_WIDTH = 220;
export const MIN_PHOTO_HEIGHT = 140;
export const MAX_PHOTO_ASPECT_RATIO = 3.0;
export const MAX_PHOTOS_PER_ENTRY = 4;

export const BULK_PHOTO_DROPBOX_URL =
  'http://goto.google.com/fcon-catalog-dropbox';
export const COSMETIC_DROPBOX_GO_LINK = ['go', 'fcon-catalog-dropbox'].join(
  '/',
);
export const PRODUCT_CATALOG_DOCS_URL =
  'http://goto.google.com/fcon-user-guide#product-catalog';
export const CAPTAIN_FIN_CHAT_URL = 'http://goto.google.com/captain-fin';
export const JETSKI_WEB_BASE_URL = 'http://goto.google.com/jetski-web';

export const ALLOWED_PLM_STATUSES = [
  'GA',
  'LA',
  'NPI',
  'EOL',
  'DEPRECATED',
  'ACTIVE',
] as const;

export interface CatalogEditableFields {
  readonly productName: string;
  readonly descriptiveName: string;
  readonly gpn: string;
  readonly resourceType: string;
  readonly fleetPlmStatus: string;
  readonly r11n: string;
  readonly numberOfDevicesPerRack: string;
  readonly unitCost: string;
}

export type CatalogEditableFieldKey = keyof CatalogEditableFields;

export const EDITABLE_SPEC_FIELD_BY_HEADER: Readonly<
  Record<string, CatalogEditableFieldKey>
> = {
  'Product Name': 'productName',
  GPN: 'gpn',
  'Descriptive Name': 'descriptiveName',
  'Resource Type': 'resourceType',
  'Fleet PLM Status': 'fleetPlmStatus',
  R11N: 'r11n',
  'Number of Devices Per Rack': 'numberOfDevicesPerRack',
  'Unit Cost': 'unitCost',
};

const ALLOWED_PHOTO_URL_PREFIX_RE =
  /^(?:https:\/\/\S+|http:\/\/(?:screen|screenshot)\/[A-Za-z0-9_.-]+|http:\/\/goto\.google\.com\/\S+|data:image\/(?:webp|png|jpeg|jpg|gif);base64,[A-Za-z0-9+/=_-]+)$/i;

const GPN_RE = /^[A-Za-z0-9][A-Za-z0-9._-]{2,31}$/;
const R11N_TOKEN_RE = /^[A-Za-z0-9._-]+$/;

/**
 * Creates the initial editable specification draft from a catalog entry.
 */
export function createCatalogEditableDraft(
  entry: UnifiedProductCatalogEntry,
): CatalogEditableFields {
  return {
    productName: entry.productName ?? '',
    descriptiveName: entry.descriptiveName ?? '',
    gpn: entry.gpn ?? '',
    resourceType: entry.resourceType ?? '',
    fleetPlmStatus: entry.fleetPlmStatus ?? '',
    r11n: (entry.r11n ?? []).join(', '),
    numberOfDevicesPerRack:
      entry.numberOfDevicesPerRack !== undefined &&
      entry.numberOfDevicesPerRack !== null
        ? String(entry.numberOfDevicesPerRack)
        : '',
    unitCost: entry.unitCost ?? '',
  };
}

/**
 * Validates a single editable specification field and returns an error string if invalid.
 */
export function validateCatalogSpecField(
  field: CatalogEditableFieldKey,
  rawValue: string,
): string | undefined {
  const trimmed = rawValue.trim();

  switch (field) {
    case 'productName':
      if (!trimmed) {
        return 'Product Name: Required field.';
      }
      if (trimmed.length > 120) {
        return 'Product Name: Must be 120 characters or fewer.';
      }
      return undefined;

    case 'descriptiveName':
      if (trimmed.length > 240) {
        return 'Descriptive Name: Must be 240 characters or fewer.';
      }
      return undefined;

    case 'gpn':
      if (!trimmed) return undefined;
      if (!GPN_RE.test(trimmed)) {
        return 'GPN: Enter a valid part number (e.g. 1055297-01).';
      }
      return undefined;

    case 'resourceType':
      if (!trimmed) {
        return 'Resource Type: Required field.';
      }
      if (trimmed.length > 64) {
        return 'Resource Type: Must be 64 characters or fewer.';
      }
      return undefined;

    case 'fleetPlmStatus':
      if (!trimmed) {
        return 'Fleet PLM Status: Required field.';
      }
      if (
        !ALLOWED_PLM_STATUSES.includes(
          trimmed.toUpperCase() as (typeof ALLOWED_PLM_STATUSES)[number],
        )
      ) {
        return `Fleet PLM Status: Must be one of ${ALLOWED_PLM_STATUSES.join(', ')}.`;
      }
      return undefined;

    case 'r11n': {
      if (!trimmed) return undefined;
      const tokens = trimmed
        .split(',')
        .map((t) => t.trim())
        .filter(Boolean);
      if (!tokens.length) {
        return 'R11N: Enter at least one valid build ID or clear the field.';
      }
      const invalid = tokens.find((t) => !R11N_TOKEN_RE.test(t));
      if (invalid) {
        return `R11N: Invalid build ID "${invalid}". Use comma-separated IDs (e.g. r11n-atb).`;
      }
      return undefined;
    }

    case 'numberOfDevicesPerRack': {
      if (!trimmed) return undefined;
      if (!/^\d+$/.test(trimmed)) {
        return 'Devices per Rack: Enter a whole number between 0 and 500.';
      }
      const num = Number(trimmed);
      if (!Number.isInteger(num) || num < 0 || num > 500) {
        return 'Devices per Rack: Enter a whole number between 0 and 500.';
      }
      return undefined;
    }

    case 'unitCost': {
      if (!trimmed) return undefined;
      const normalized = trimmed.replace(/^\$/, '').replace(/,/g, '').trim();
      const num = Number(normalized);
      if (!normalized || Number.isNaN(num) || num < 0 || num > 10_000_000) {
        return 'Unit Cost: Enter a valid non-negative USD amount (e.g. 26645).';
      }
      return undefined;
    }
  }
}

function normalizeCatalogSpecValue(
  key: CatalogEditableFieldKey,
  rawValue: string,
): string {
  const trimmed = rawValue.trim();
  if (!trimmed) return '';
  switch (key) {
    case 'fleetPlmStatus':
      return trimmed.toUpperCase();
    case 'unitCost':
      return trimmed.replace(/^\$/, '').replace(/,/g, '').trim();
    case 'r11n':
      return trimmed
        .split(',')
        .map((t) => t.trim())
        .filter(Boolean)
        .join(', ');
    default:
      return trimmed;
  }
}

/**
 * Validates modified specification fields and returns a map of field errors.
 * Fields whose normalized value matches the baseline are skipped so pre-existing
 * empty or non-standard values never trigger false-positive errors.
 */
export function validateAllCatalogSpecFields(
  draft: CatalogEditableFields,
  baseline?: CatalogEditableFields,
): Partial<Record<CatalogEditableFieldKey, string>> {
  const errors: Partial<Record<CatalogEditableFieldKey, string>> = {};
  (Object.keys(draft) as CatalogEditableFieldKey[]).forEach((key) => {
    if (
      baseline &&
      Boolean(draft[key].trim()) === Boolean(baseline[key].trim()) &&
      normalizeCatalogSpecValue(key, draft[key]) ===
        normalizeCatalogSpecValue(key, baseline[key])
    ) {
      return;
    }
    const err = validateCatalogSpecField(key, draft[key]);
    if (err) {
      errors[key] = err;
    }
  });
  return errors;
}

/**
 * Computes field-level diffs between the original catalog entry and the edited specification draft.
 */
export function computeCatalogSpecDiffs(
  entry: UnifiedProductCatalogEntry,
  draft: CatalogEditableFields,
): CatalogDiffItem[] {
  const baseline = createCatalogEditableDraft(entry);
  const diffs: CatalogDiffItem[] = [];

  const fieldLabels: Record<CatalogEditableFieldKey, string> = {
    productName: 'Product Name',
    descriptiveName: 'Descriptive Name',
    gpn: 'GPN',
    resourceType: 'Resource Type',
    fleetPlmStatus: 'Fleet PLM Status',
    r11n: 'R11N',
    numberOfDevicesPerRack: 'Number of Devices Per Rack',
    unitCost: 'Unit Cost',
  };

  (Object.keys(fieldLabels) as CatalogEditableFieldKey[]).forEach((key) => {
    const beforeVal = normalizeCatalogSpecValue(key, baseline[key]);
    const afterVal = normalizeCatalogSpecValue(key, draft[key]);
    if (beforeVal !== afterVal) {
      diffs.push({
        label: fieldLabels[key],
        before: beforeVal || '—',
        after: afterVal || '—',
      });
    }
  });

  return diffs;
}

/**
 * Validates a candidate reference photo URL before staging it.
 */
export function validatePhotoUrlInput(
  rawUrl: string,
  existingUrls: readonly string[],
): string | undefined {
  const trimmed = rawUrl.trim();
  if (!trimmed) {
    return 'Enter an image URL (e.g. http://screen/..., https://drive.google.com/..., or https://...).';
  }
  if (existingUrls.length >= MAX_PHOTOS_PER_ENTRY) {
    return `Maximum of ${MAX_PHOTOS_PER_ENTRY} reference photos per catalog entry.`;
  }
  if (!ALLOWED_PHOTO_URL_PREFIX_RE.test(trimmed)) {
    return 'Use an https:// image URL, http://screen/<id> screenshot link, or data:image/ URI.';
  }
  if (existingUrls.includes(trimmed)) {
    return 'This photo URL is already staged on this catalog entry.';
  }
  if (trimmed.toLowerCase().includes('1u_strip')) {
    return '1U rack-strip diagrams stretch in card view. Provide a device or rack photo URL (>= 220×140 px, aspect <= 3.0:1).';
  }
  return undefined;
}

/**
 * Validates loaded image dimensions to reject tiny icons and 1U rack-strip diagrams.
 */
export function validatePhotoDimensions(
  width: number,
  height: number,
): string | undefined {
  if (width <= 0 || height <= 0) {
    return 'Could not read image dimensions.';
  }
  if (width < MIN_PHOTO_WIDTH || height < MIN_PHOTO_HEIGHT) {
    return `Image is ${width}×${height} px. Minimum size is ${MIN_PHOTO_WIDTH}×${MIN_PHOTO_HEIGHT} px.`;
  }
  const ratio = width / height;
  if (ratio > MAX_PHOTO_ASPECT_RATIO) {
    return `Image aspect ratio (${ratio.toFixed(1)}:1) exceeds ${MAX_PHOTO_ASPECT_RATIO.toFixed(1)}:1. Avoid 1U rack-strip diagrams.`;
  }
  return undefined;
}

/**
 * Returns true if two ordered photo URI lists are identical.
 */
export function arePhotoListsEqual(
  a: readonly string[],
  b: readonly string[],
): boolean {
  if (a.length !== b.length) return false;
  return a.every((val, idx) => val === b[idx]);
}

/**
 * Builds a pre-filled Jetski Web composer URL using the `?q=` query parameter.
 */
export function buildJetskiPrefilledPromptUrl(prompt: string): string {
  return `${JETSKI_WEB_BASE_URL}?q=${encodeURIComponent(prompt)}`;
}

/**
 * Builds a prompt for syncing bulk photos uploaded to the Google Drive Drop-Box.
 */
export function buildBulkDropboxSyncPrompt(productCatalogId: string): string {
  return (
    `I uploaded new hardware photos into ${BULK_PHOTO_DROPBOX_URL} (including ${productCatalogId}). ` +
    'Please use the fcon-product-catalog skill to sync the drop-box photos, convert them to .webp, ' +
    'run lucicfg, and upload a Gerrit CL for review.'
  );
}

/**
 * Builds a deterministic prompt for updating Product Catalog photos and/or metadata specs via Jetski or Captain Fin.
 */
export function buildCatalogPhotoEditPrompt(
  entry: UnifiedProductCatalogEntry,
  stagedPhotos: readonly string[],
  specDiffs: readonly CatalogDiffItem[] = [],
  reasonNote?: string,
): string {
  const originalPhotos = entry.photoDataUris ?? [];
  const label =
    entry.productName || entry.descriptiveName || entry.productCatalogId;
  const category = entry.productType || 'hardware';
  const hasPhotoChanges = !arePhotoListsEqual(originalPhotos, stagedPhotos);

  const lines: string[] = [
    [
      `Please update Fleet Console Product Catalog entry ${entry.productCatalogId} ("${label}", category: ${category})`,
      'in chrome-internal/infradata/config across both configs/fleet-console-dev/products/ and',
      'configs/fleet-console-prod/products/ using the fcon-product-catalog skill:',
    ].join(' '),
  ];

  if (specDiffs.length > 0) {
    lines.push('', 'Specification Field Updates:');
    specDiffs.forEach((diff) => {
      lines.push(`- ${diff.label}: ${diff.before} -> ${diff.after}`);
    });
  }

  if (hasPhotoChanges) {
    const prevSummary =
      originalPhotos.length > 0
        ? originalPhotos
            .map((u, i) =>
              u.startsWith('data:image/')
                ? `existing_photo_${i + 1}.webp (inline catalog image)`
                : u,
            )
            .join(', ')
        : 'None';

    const nextLines =
      stagedPhotos.length > 0
        ? stagedPhotos.map((u, i) => {
            const origIdx = originalPhotos.indexOf(u);
            const displayUrl = u.startsWith('data:image/')
              ? origIdx !== -1
                ? `existing_photo_${origIdx + 1}.webp (retained catalog image)`
                : `new_photo_${i + 1}.png (upload to ${COSMETIC_DROPBOX_GO_LINK} as ${entry.productCatalogId}_${i + 1}.png)`
              : u;
            return `  ${i + 1}. ${displayUrl}${i === 0 ? ' (primary hero photo)' : ''}`;
          })
        : [
            '  (Remove all reference photos — revert to category blueprint fallback)',
          ];

    lines.push(
      '',
      'Reference Photos Update:',
      `- Previous photos (${originalPhotos.length}): ${prevSummary}`,
      `- Updated photos (${stagedPhotos.length}):`,
      ...nextLines,
    );
  }

  if (reasonNote && reasonNote.trim()) {
    lines.push('', `Reason: ${reasonNote.trim()}`);
  }

  lines.push(
    '',
    [
      'Please update the Starlark entry in both fleet-console-dev and fleet-console-prod, fetch and convert',
      'any new photo URLs to .webp (>= 220x140 px), run lucicfg to regenerate config.cfg and photos.cfg,',
      'and upload a Gerrit CL for review.',
    ].join(' '),
  );

  return lines.join('\n');
}
