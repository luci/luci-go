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

export const isTyping = (activeElement: Element | null) => {
  if (!activeElement) return false;

  const tagName = activeElement.tagName.toLowerCase();

  // 1. Check for standard text inputs
  const isTextInput =
    activeElement instanceof HTMLInputElement &&
    /^(text|email|password|search|tel|url|number)$/i.test(activeElement.type);

  const isTextArea = tagName === 'textarea';

  const isRichText =
    (activeElement as HTMLElement).isContentEditable ||
    activeElement.getAttribute('contenteditable') === 'true' ||
    activeElement.getAttribute('contenteditable') === '';

  const isInsideModal = activeElement.closest('[role="dialog"]') !== null;

  const isReadOnly =
    'readOnly' in activeElement && activeElement.readOnly === true;
  const isDisabled =
    'disabled' in activeElement && activeElement.disabled === true;

  const typing =
    (isTextInput || isTextArea || isRichText) && !isReadOnly && !isDisabled;

  return typing || isInsideModal;
};
