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

import { act, renderHook } from '@testing-library/react';

import { UnifiedProductCatalogEntry } from './types';
import { useCatalogEditState } from './use_catalog_edit_state';
import { useCatalogPhotoEditState } from './use_catalog_photo_edit_state';
import { useCatalogSpecEditState } from './use_catalog_spec_edit_state';

const MOCK_ENTRY: UnifiedProductCatalogEntry = {
  productCatalogId: 'ATB-006',
  productName: 'Android OmniLab Testbed',
  gpn: '1055297-01',
  descriptiveName: '16-slot enclosure',
  resourceType: 'TESTBED',
  fleetPlmStatus: 'GA',
  r11n: ['r11n-atb'],
  numberOfDevicesPerRack: 4,
  unitCost: '26645',
  productType: 'android-testbed',
  photoDataUris: ['https://example.com/photo1.png'],
};

describe('useCatalogSpecEditState', () => {
  it('tracks specification field edits, validation errors, and reset', () => {
    const { result } = renderHook(() =>
      useCatalogSpecEditState('ATB-006', MOCK_ENTRY),
    );

    expect(result.current.specDiffs).toHaveLength(0);
    expect(result.current.activeSpecDraft.unitCost).toBe('26645');

    act(() => {
      result.current.handleSpecFieldChange('unitCost', '-5');
    });
    expect(result.current.specErrorMessages).toHaveLength(1);

    act(() => {
      result.current.handleResetSpecs();
    });
    expect(result.current.specErrorMessages).toHaveLength(0);
    expect(result.current.displayEntry?.unitCost).toBe('26645');
  });
});

describe('useCatalogPhotoEditState', () => {
  it('stages, reorders, bounds-checks removal, and tracks dimension errors', () => {
    const { result } = renderHook(() =>
      useCatalogPhotoEditState('ATB-006', MOCK_ENTRY),
    );

    act(() => {
      result.current.handlePhotoUrlDraftChange(
        'https://example.com/photo2.png',
      );
    });
    act(() => {
      result.current.handleStagePhotoUrl();
    });

    expect(result.current.stagedPhotos).toEqual([
      'https://example.com/photo1.png',
      'https://example.com/photo2.png',
    ]);
    expect(result.current.hasPhotoChanges).toBe(true);

    // Out-of-bounds removal should be a no-op.
    act(() => {
      result.current.handleRemoveStagedPhoto(-1);
      result.current.handleRemoveStagedPhoto(99);
    });
    expect(result.current.stagedPhotos).toHaveLength(2);

    act(() => {
      result.current.handleSetPrimaryPhoto(1);
    });
    expect(result.current.stagedPhotos[0]).toBe(
      'https://example.com/photo2.png',
    );
  });
});

describe('useCatalogEditState', () => {
  it('composes photo and spec edit state and resets on entry switch', () => {
    const { result, rerender } = renderHook(
      ({ id, entry }) => useCatalogEditState(id, entry),
      { initialProps: { id: 'ATB-006', entry: MOCK_ENTRY } },
    );

    expect(result.current.dirtyCount).toBe(0);

    act(() => {
      result.current.handleSpecFieldChange('unitCost', '30000');
    });
    expect(result.current.dirtyCount).toBe(1);
    expect(result.current.displayEntry?.unitCost).toBe('30000');

    // Switching catalog entry id resets staged overrides.
    rerender({ id: 'ATB-007', entry: MOCK_ENTRY });
    expect(result.current.dirtyCount).toBe(0);
    expect(result.current.displayEntry?.unitCost).toBe('26645');
  });
});
