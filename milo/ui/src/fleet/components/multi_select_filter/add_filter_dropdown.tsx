// Copyright 2024 The LUCI Authors.
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

import ArrowRightIcon from '@mui/icons-material/ArrowRight';
import { Backdrop, Card, Checkbox, MenuItem, MenuList } from '@mui/material';
import _ from 'lodash';
import { useEffect, useMemo, useRef, useState } from 'react';

import { Option, SelectedOptions } from '@/fleet/types';

import { fuzzySort, hasAnyModifier, keyboardUpDownHandler } from '../../utils';
import { HighlightCharacter } from '../highlight_character';
import { Footer } from '../options_dropdown/footer';
import { SearchInput } from '../search_input';

export function AddFilterDropdown({
  filterOptions,
  selectedOptions: initSelectedOptions,
  setSelectedOptions,
  anchorEl,
  setAnchorEL,
}: {
  filterOptions: Option[];
  selectedOptions: SelectedOptions;
  setSelectedOptions: React.Dispatch<React.SetStateAction<SelectedOptions>>;
  anchorEl: HTMLElement | null;
  setAnchorEL: React.Dispatch<React.SetStateAction<HTMLElement | null>>;
}) {
  const [anchorElInner, setAnchorELInner] = useState<HTMLElement | null>(null);
  const [openCategoryIndex, setOpenCategory] = useState<number | undefined>(
    undefined,
  );
  const searchInput = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (anchorEl) {
      searchInput.current?.focus();
    }
  }, [anchorEl]);

  const [searchQuery, setSearchQuery] = useState('');

  const cardRef = useRef<HTMLDivElement>(null);
  const innerCardRef = useRef<HTMLDivElement>(null);

  const [tempSelectedOptions, setTempSelectedOptions] =
    useState(initSelectedOptions);

  const flipOption = (parentValue: string, o2Value: string) => {
    const currentValues = tempSelectedOptions[parentValue] ?? [];

    const newValues = currentValues.includes(o2Value)
      ? currentValues.filter((v) => v !== o2Value)
      : currentValues.concat(o2Value);

    setTempSelectedOptions({
      ...(tempSelectedOptions ?? {}),
      [parentValue]: newValues,
    });
  };

  useEffect(() => {
    setTempSelectedOptions(initSelectedOptions);
  }, [initSelectedOptions, openCategoryIndex]);

  if (cardRef.current) {
    const anchorRect = anchorEl?.getBoundingClientRect();
    if (anchorRect) {
      cardRef.current.style.left = `${anchorRect.width / 4}px`;
      cardRef.current.style.top = `${anchorRect.height}px`;
    }
  }

  if (innerCardRef.current) {
    const anchorRect = anchorEl?.getBoundingClientRect();
    const outerMenuRect = anchorElInner?.getBoundingClientRect();
    if (outerMenuRect && anchorRect) {
      innerCardRef.current.style.left = `${anchorRect.width / 4 + outerMenuRect.width + 2}px`;
      if (outerMenuRect.top > window.innerHeight / 2) {
        innerCardRef.current.style.top = '';
        innerCardRef.current.style.bottom = `${-outerMenuRect.bottom + anchorRect.top - 10}px`;
      } else {
        innerCardRef.current.style.bottom = '';
        innerCardRef.current.style.top = `${outerMenuRect.top - anchorRect.top - 10}px`;
      }
    }
  }

  const closeInnerMenu = () => {
    setOpenCategory(undefined);
    anchorElInner?.focus();
    setAnchorELInner(null);
  };
  const closeMenu = () => {
    setSearchQuery('');
    closeInnerMenu();
    setAnchorEL(null);
  };

  const filterResults = useMemo(
    () =>
      Object.values(
        _.groupBy(
          fuzzySort(searchQuery)(
            filterOptions,
            (el) => el.options,
            (el) => el.label,
          ),
          (o) => o.parent?.value ?? o.el.value,
        ),
      ),
    [filterOptions, searchQuery],
  );

  const applyOptions = () => {
    closeMenu();
    setSelectedOptions(tempSelectedOptions);
  };

  const handleRandomTextInput = (e: React.KeyboardEvent<HTMLUListElement>) => {
    // allow user to search when any alphanumeric key has been pressed
    if (/^[a-zA-Z0-9]\b/.test(e.key) && !hasAnyModifier(e)) {
      closeInnerMenu();
      searchInput.current?.focus();
      setSearchQuery((old) => old + e.key);
      e.preventDefault(); // Avoid race condition to type twice in the input
    }
  };

  return (
    <>
      <Backdrop
        open={!!anchorEl}
        onClick={() => {
          closeMenu();
        }}
        invisible={true}
        sx={{
          zIndex: 2,
        }}
      ></Backdrop>
      <div
        css={{
          zIndex: 3,
          position: 'absolute',
          display: anchorEl === null ? 'none' : 'block',
        }}
      >
        <Card
          elevation={2}
          onClick={(e) => e.stopPropagation()}
          sx={{ position: 'absolute' }}
          ref={cardRef}
        >
          <MenuList
            sx={{
              minWidth: 200,
              maxHeight: 300,
              overflow: 'auto',
            }}
            onKeyDown={(e) => {
              keyboardUpDownHandler(e);
              switch (e.key) {
                case 'Delete':
                case 'Cancel':
                case 'Backspace':
                  setSearchQuery('');
                  searchInput.current?.focus();
                  break;
                case 'Escape':
                  closeMenu();
              }

              handleRandomTextInput(e);
            }}
          >
            <SearchInput
              searchInput={searchInput}
              searchQuery={searchQuery}
              onChange={(e) => {
                setSearchQuery(e.currentTarget.value);
              }}
            />
            {filterResults.map((searchResult, idx) => {
              const parent = searchResult[0].parent ?? searchResult[0].el;
              const parentMatches = searchResult
                .filter((sr) => sr.parent === undefined)
                .at(0)?.matches;

              return (
                <MenuItem
                  onClick={(event) => {
                    // The onClick fires also when closing the menu
                    if (openCategoryIndex !== idx) {
                      setOpenCategory(idx);
                      setAnchorELInner(event.currentTarget);
                    } else {
                      setOpenCategory(undefined);
                      setAnchorELInner(null);
                    }
                  }}
                  onKeyDown={(e) => {
                    if (e.key === 'ArrowRight') {
                      e.currentTarget.click();
                    }
                  }}
                  key={`item-${parent.value}-${idx}`}
                  disableRipple
                  selected={openCategoryIndex === idx}
                  sx={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    width: '100%',
                    minHeight: 'auto',
                  }}
                >
                  <HighlightCharacter
                    variant="body2"
                    highlightIndexes={parentMatches}
                  >
                    {parent.label}
                  </HighlightCharacter>
                  <ArrowRightIcon />
                </MenuItem>
              );
            })}
          </MenuList>
        </Card>
        <div ref={innerCardRef} css={{ position: 'absolute' }}>
          {anchorElInner &&
            innerCardRef.current &&
            openCategoryIndex !== undefined && (
              <Card onClick={(e) => e.stopPropagation()}>
                <MenuList
                  variant="selectedMenu"
                  sx={{ overflowY: 'hidden', overflow: 'auto', maxHeight: 400 }}
                  onKeyDown={(e: React.KeyboardEvent<HTMLUListElement>) => {
                    if (e.key === 'Tab') {
                      closeMenu();
                    }
                    if (e.key === 'Escape' || e.key === 'ArrowLeft') {
                      closeInnerMenu();
                    }
                    if (e.key === 'Enter' && e.ctrlKey) {
                      applyOptions();
                    }
                    if (
                      e.key === 'Delete' ||
                      e.key === 'Backspace' ||
                      e.key === 'Cancel'
                    ) {
                      setSearchQuery('');
                      searchInput.current?.focus();
                    }

                    handleRandomTextInput(e);
                  }}
                >
                  {filterResults[openCategoryIndex]
                    .filter((sr) => sr.parent !== undefined)
                    .map((o2, idx) => (
                      <MenuItem
                        key={`innerMenu-${o2.parent!.value}-${o2.el.value}`}
                        disableRipple
                        onClick={(e) => {
                          if (e.type === 'keydown' || e.type === 'keyup') {
                            const parsedE =
                              e as unknown as React.KeyboardEvent<HTMLLIElement>;
                            if (parsedE.key === ' ') return;
                            if (parsedE.key === 'Enter' && parsedE.ctrlKey)
                              return;
                          }
                          flipOption(o2.parent!.value, o2.el.value);
                        }}
                        onKeyDown={keyboardUpDownHandler}
                        // eslint-disable-next-line jsx-a11y/no-autofocus
                        autoFocus={idx === 0}
                        sx={{
                          display: 'flex',
                          alignItems: 'center',
                          padding: '6px 12px',
                          minHeight: 'auto',
                        }}
                      >
                        <Checkbox
                          sx={{
                            padding: 0,
                            marginRight: '13px',
                          }}
                          size="small"
                          checked={
                            !!tempSelectedOptions[o2.parent!.value]?.includes(
                              o2.el.value,
                            )
                          }
                          tabIndex={-1}
                        />
                        <HighlightCharacter
                          variant="body2"
                          highlightIndexes={o2.matches}
                        >
                          {o2.label}
                        </HighlightCharacter>
                      </MenuItem>
                    ))}
                </MenuList>
                <Footer onCancelClick={closeMenu} onApplyClick={applyOptions} />
              </Card>
            )}
        </div>
      </div>
    </>
  );
}
