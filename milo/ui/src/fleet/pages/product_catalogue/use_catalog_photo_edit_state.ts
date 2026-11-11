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

import {
  arePhotoListsEqual,
  normalizePhotoUrlInput,
  validatePhotoDimensions,
  validatePhotoUrlInput,
} from './catalog_edit_validation';
import { UnifiedProductCatalogEntry } from './types';

const EMPTY_PHOTOS: readonly string[] = [];

function updateDimensionErrorRecord(
  prev: Record<string, string>,
  uri: string,
  errorMsg: string | undefined,
): Record<string, string> {
  if (errorMsg ? prev[uri] === errorMsg : !prev[uri]) {
    return prev;
  }
  const next = { ...prev };
  if (errorMsg) {
    next[uri] = errorMsg;
  } else {
    delete next[uri];
  }
  return next;
}

export interface UseCatalogPhotoEditStateResult {
  readonly originalPhotos: readonly string[];
  readonly stagedPhotos: readonly string[];
  readonly hasPhotoChanges: boolean;
  readonly isPhotoEditorOpen: boolean;
  readonly photoUrlDraft: string;
  readonly photoInputError: string | undefined;
  readonly photoDimensionErrors: Record<string, string>;
  readonly activeDimensionErrorMessages: readonly string[];
  readonly handleOpenPhotoEditor: () => void;
  readonly handleClosePhotoEditor: () => void;
  readonly handlePhotoUrlDraftChange: (val: string) => void;
  readonly handleStagePhotoUrl: () => boolean;
  readonly handleRemoveStagedPhoto: (idx: number) => void;
  readonly handleSetPrimaryPhoto: (idx: number) => void;
  readonly handleResetPhotos: () => void;
  readonly handlePhotoDimensionsEvaluated: (
    uri: string,
    width: number,
    height: number,
  ) => void;
}

export const useCatalogPhotoEditState = (
  id: string,
  entry?: UnifiedProductCatalogEntry,
): UseCatalogPhotoEditStateResult => {
  const originalPhotos = useMemo(
    () => entry?.photoDataUris ?? EMPTY_PHOTOS,
    [entry?.photoDataUris],
  );

  const [isPhotoEditorOpen, setIsPhotoEditorOpen] = useState(false);
  const [stagedPhotosOverride, setStagedPhotosOverride] = useState<
    readonly string[] | undefined
  >(undefined);
  const [photoUrlDraft, setPhotoUrlDraft] = useState('');
  const [photoInputError, setPhotoInputError] = useState<string | undefined>(
    undefined,
  );
  const [photoDimensionErrors, setPhotoDimensionErrors] = useState<
    Record<string, string>
  >({});

  const handleResetPhotos = useCallback(() => {
    setStagedPhotosOverride(undefined);
    setPhotoUrlDraft('');
    setPhotoInputError(undefined);
    setPhotoDimensionErrors({});
  }, []);

  useEffect(() => {
    handleResetPhotos();
    setIsPhotoEditorOpen(false);
  }, [handleResetPhotos, id]);

  const stagedPhotos = stagedPhotosOverride ?? originalPhotos;
  const hasPhotoChanges =
    entry !== undefined && !arePhotoListsEqual(originalPhotos, stagedPhotos);

  const activeDimensionErrorMessages = useMemo(
    () =>
      stagedPhotos
        .filter(
          (uri) =>
            Boolean(photoDimensionErrors[uri]) && !originalPhotos.includes(uri),
        )
        .map((uri) => photoDimensionErrors[uri]),
    [originalPhotos, photoDimensionErrors, stagedPhotos],
  );

  const handleOpenPhotoEditor = useCallback(() => {
    setIsPhotoEditorOpen(true);
  }, []);

  const handleClosePhotoEditor = useCallback(() => {
    setIsPhotoEditorOpen(false);
  }, []);

  const handlePhotoUrlDraftChange = useCallback((val: string) => {
    setPhotoUrlDraft(val);
    setPhotoInputError(undefined);
  }, []);

  const handleStagePhotoUrl = useCallback((): boolean => {
    const err = validatePhotoUrlInput(photoUrlDraft, stagedPhotos);
    if (err) {
      setPhotoInputError(err);
      return false;
    }
    setStagedPhotosOverride([
      ...stagedPhotos,
      normalizePhotoUrlInput(photoUrlDraft),
    ]);
    setPhotoUrlDraft('');
    setPhotoInputError(undefined);
    return true;
  }, [photoUrlDraft, stagedPhotos]);

  const handleRemoveStagedPhoto = useCallback(
    (idx: number) => {
      if (idx < 0 || idx >= stagedPhotos.length) return;
      const removedUri = stagedPhotos[idx];
      setStagedPhotosOverride(stagedPhotos.filter((_, i) => i !== idx));
      setPhotoInputError(undefined);
      if (removedUri) {
        setPhotoDimensionErrors((prev) =>
          updateDimensionErrorRecord(prev, removedUri, undefined),
        );
      }
    },
    [stagedPhotos],
  );

  const handleSetPrimaryPhoto = useCallback(
    (idx: number) => {
      if (idx <= 0 || idx >= stagedPhotos.length) return;
      const target = stagedPhotos[idx];
      const rest = stagedPhotos.filter((_, i) => i !== idx);
      setStagedPhotosOverride([target, ...rest]);
    },
    [stagedPhotos],
  );

  const handlePhotoDimensionsEvaluated = useCallback(
    (uri: string, width: number, height: number) => {
      const dimErr = validatePhotoDimensions(width, height);
      setPhotoDimensionErrors((prev) =>
        updateDimensionErrorRecord(prev, uri, dimErr),
      );
    },
    [],
  );

  return useMemo(
    () => ({
      originalPhotos,
      stagedPhotos,
      hasPhotoChanges,
      isPhotoEditorOpen,
      photoUrlDraft,
      photoInputError,
      photoDimensionErrors,
      activeDimensionErrorMessages,
      handleOpenPhotoEditor,
      handleClosePhotoEditor,
      handlePhotoUrlDraftChange,
      handleStagePhotoUrl,
      handleRemoveStagedPhoto,
      handleSetPrimaryPhoto,
      handleResetPhotos,
      handlePhotoDimensionsEvaluated,
    }),
    [
      originalPhotos,
      stagedPhotos,
      hasPhotoChanges,
      isPhotoEditorOpen,
      photoUrlDraft,
      photoInputError,
      photoDimensionErrors,
      activeDimensionErrorMessages,
      handleOpenPhotoEditor,
      handleClosePhotoEditor,
      handlePhotoUrlDraftChange,
      handleStagePhotoUrl,
      handleRemoveStagedPhoto,
      handleSetPrimaryPhoto,
      handleResetPhotos,
      handlePhotoDimensionsEvaluated,
    ],
  );
};
