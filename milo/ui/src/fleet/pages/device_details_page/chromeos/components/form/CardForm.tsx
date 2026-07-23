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

/* eslint-disable react-refresh/only-export-components */
import {
  createContext,
  useContext,
  useState,
  ReactNode,
  useEffect,
} from 'react';

import { getNestedValue } from '../../utils/inventory_editing_utils';
import { InventoryDataCard } from '../common/InventoryDataCard';

import { useInventoryForm } from './InventoryFormContext';

interface CardFormContextType {
  isEditing: boolean;
  getFieldValue: (path: string | string[]) => unknown;
  setFieldValue: (path: string | string[], value: unknown) => void;
  confirm: () => void;
}

const CardFormContext = createContext<CardFormContextType | null>(null);

export const useCardForm = () => {
  const context = useContext(CardFormContext);
  if (!context) {
    throw new Error('useCardForm must be used within a CardForm provider');
  }
  return context;
};

interface CardFormProps {
  cardId: string;
  title: string;
  isEmpty: boolean;
  emptyMessage: string;
  children: ReactNode;
}

export const CardForm = ({
  cardId,
  title,
  isEmpty,
  emptyMessage,
  children,
}: CardFormProps) => {
  const {
    draftLse,
    updateDraftFields,
    activeEditingCardId,
    setActiveEditingCardId,
    editable,
  } = useInventoryForm();

  const isEditing = activeEditingCardId === cardId;
  const [stagedUpdates, setStagedUpdates] = useState<Record<string, unknown>>(
    {},
  );

  useEffect(() => {
    if (!isEditing) {
      setStagedUpdates({});
    }
  }, [isEditing]);

  const getFieldValue = (path: string | string[]) => {
    const key = typeof path === 'string' ? path : path.join('.');
    if (isEditing && key in stagedUpdates) {
      return stagedUpdates[key];
    }
    return getNestedValue(draftLse, path);
  };

  const setFieldValue = (path: string | string[], value: unknown) => {
    const key = typeof path === 'string' ? path : path.join('.');
    setStagedUpdates((prev) => ({ ...prev, [key]: value }));
  };

  const handleEdit = () => {
    setStagedUpdates({});
    setActiveEditingCardId(cardId);
  };

  const handleConfirm = () => {
    const updates = Object.entries(stagedUpdates).map(([key, value]) => ({
      path: key,
      value,
    }));
    if (updates.length > 0) {
      updateDraftFields(updates);
    }
    setStagedUpdates({});
    setActiveEditingCardId(null);
  };

  const handleCancel = () => {
    setStagedUpdates({});
    setActiveEditingCardId(null);
  };

  return (
    <CardFormContext.Provider
      value={{
        isEditing,
        getFieldValue,
        setFieldValue,
        confirm: handleConfirm,
      }}
    >
      <InventoryDataCard
        title={title}
        emptyMessage={isEmpty && !isEditing ? emptyMessage : undefined}
        editable={
          editable &&
          (activeEditingCardId === null || activeEditingCardId === cardId)
        }
        isEditing={isEditing}
        onEdit={handleEdit}
        onConfirm={handleConfirm}
        onDiscard={handleCancel}
      >
        {children}
      </InventoryDataCard>
    </CardFormContext.Provider>
  );
};
