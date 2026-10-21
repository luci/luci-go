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

const ALLOWED_PHOTO_URL_PREFIX_RE =
  /^(?:https:\/\/\S+|http:\/\/(?:screen|screenshot)\/[A-Za-z0-9_.-]+|http:\/\/goto\.google\.com\/\S+|data:image\/(?:webp|png|jpeg|jpg|gif);base64,[A-Za-z0-9+/=_-]+)$/i;

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
 * Builds a deterministic prompt for updating Product Catalog photos via Jetski or Captain Fin.
 */
export function buildCatalogPhotoEditPrompt(
  entry: UnifiedProductCatalogEntry,
  stagedPhotos: readonly string[],
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
