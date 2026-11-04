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

import { useCallback, useEffect, useMemo, useState } from 'react';

import { copyTextToClipboard } from './catalog_details_helpers';
import { buildCatalogPhotoEditPrompt } from './catalog_edit_validation';
import { UnifiedProductCatalogEntry } from './types';
import {
  useCatalogPhotoEditState,
  UseCatalogPhotoEditStateResult,
} from './use_catalog_photo_edit_state';
import {
  useCatalogSpecEditState,
  UseCatalogSpecEditStateResult,
} from './use_catalog_spec_edit_state';

export interface UseCatalogEditStateResult
  extends UseCatalogPhotoEditStateResult,
    UseCatalogSpecEditStateResult {
  readonly dirtyCount: number;
  readonly validationErrorCount: number;
  readonly firstValidationErrorMessage: string | undefined;
  readonly copiedPrompt: boolean;
  readonly generatedPrompt: string;
  readonly handleDiscardEdits: () => void;
  readonly handleCopyPrompt: () => Promise<void>;
}

export const useCatalogEditState = (
  id: string,
  entry?: UnifiedProductCatalogEntry,
): UseCatalogEditStateResult => {
  const photoState = useCatalogPhotoEditState(id, entry);
  const specState = useCatalogSpecEditState(id, entry);
  const [copiedPrompt, setCopiedPrompt] = useState(false);

  const dirtyCount =
    specState.specDiffs.length + (photoState.hasPhotoChanges ? 1 : 0);
  const validationErrorCount =
    photoState.activeDimensionErrorMessages.length +
    specState.specErrorMessages.length;
  const firstValidationErrorMessage =
    specState.specErrorMessages[0] ??
    photoState.activeDimensionErrorMessages[0];

  const generatedPrompt = useMemo(() => {
    if (!entry) return '';
    return buildCatalogPhotoEditPrompt(
      entry,
      photoState.stagedPhotos,
      specState.specDiffs,
    );
  }, [entry, photoState.stagedPhotos, specState.specDiffs]);

  useEffect(() => {
    setCopiedPrompt(false);
  }, [generatedPrompt, id]);

  const handleResetPhotos = useCallback(() => {
    photoState.handleResetPhotos();
    setCopiedPrompt(false);
  }, [photoState]);

  const handleDiscardEdits = useCallback(() => {
    photoState.handleResetPhotos();
    specState.handleResetSpecs();
    setCopiedPrompt(false);
  }, [photoState, specState]);

  const handleCopyPrompt = useCallback(async () => {
    const ok = await copyTextToClipboard(generatedPrompt);
    setCopiedPrompt(ok);
  }, [generatedPrompt]);

  return useMemo(
    () => ({
      ...photoState,
      ...specState,
      dirtyCount,
      validationErrorCount,
      firstValidationErrorMessage,
      copiedPrompt,
      generatedPrompt,
      handleResetPhotos,
      handleDiscardEdits,
      handleCopyPrompt,
    }),
    [
      photoState,
      specState,
      dirtyCount,
      validationErrorCount,
      firstValidationErrorMessage,
      copiedPrompt,
      generatedPrompt,
      handleResetPhotos,
      handleDiscardEdits,
      handleCopyPrompt,
    ],
  );
};
