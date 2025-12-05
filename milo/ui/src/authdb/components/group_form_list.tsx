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

import '@/authdb/components/groups.css';

import AddCircleIcon from '@mui/icons-material/AddCircle';
import ExpandLessIcon from '@mui/icons-material/ExpandLess';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import RemoveCircleOutlineIcon from '@mui/icons-material/RemoveCircleOutline';
import Button from '@mui/material/Button';
import Checkbox from '@mui/material/Checkbox';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogContentText from '@mui/material/DialogContentText';
import DialogTitle from '@mui/material/DialogTitle';
import IconButton from '@mui/material/IconButton';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableRow from '@mui/material/TableRow';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import {
  useState,
  forwardRef,
  useImperativeHandle,
  useEffect,
  useCallback,
} from 'react';

import {
  addPrefixToItems,
  isGlob,
  isMember,
  isSubgroup,
  stripPrefixFromItems,
  userPrefix,
} from '@/authdb/common/helpers';
import { GroupLink } from '@/authdb/components/group_link';
import { AuthLookupLink } from '@/authdb/components/lookup_link';

const expansionThreshold = 10;

interface GroupFormListProps {
  // Sets the starting items array. Used on initial GetGroup call from
  // groups_form.
  initialValues: string[];
  // This will be either members, subgroups or globs. Used for header in form
  // and to check validity of added items.
  name: string;
  // Used on UpdateGroup call to backend to update this field's values.
  submitValues: () => void;
}

export interface FormListElement {
  getItems: () => string[];
  changeItems: (items: string[]) => void;
  resetToSavedValues: () => void;
}

type Item = {
  value: string;
  checked: boolean;
};

// Converts string (of item names) array to Item array.
const asItems = (values: string[]) => {
  const items: Item[] = [];
  values.forEach((item) => {
    items.push({
      value: item,
      checked: false,
    });
  });
  return items;
};

const asString = (items: Item[]) => {
  const itemValues: string[] = [];
  items.forEach((item) => {
    itemValues.push(item.value);
  });
  return itemValues;
};

export const GroupFormList = forwardRef<FormListElement, GroupFormListProps>(
  ({ initialValues, name, submitValues }, ref) => {
    const [addingItem, setAddingItem] = useState(false);
    const [newItems, setNewItems] = useState('');
    const [invalidValueError, setInvalidValueError] = useState('');
    const [duplicateValueError, setDuplicateValueError] = useState('');

    // Display items alphabetically.
    initialValues.sort();
    // The initial form items which reflect the items currently in auth service backend.
    const [savedValues, setSavedValues] = useState<string[]>(initialValues);
    // The current edited item list, including removed & added items.
    const [items, setItems] = useState<Item[]>(asItems(initialValues));
    const [removeDialogVisible, setRemoveDialogVisible] = useState<boolean>();
    const [expanded, setExpanded] = useState<boolean>(true);

    let placeHolderText: string;
    switch (name) {
      case 'Members':
        placeHolderText =
          'Add members, one per line (e.g. person@example.com, serviceAccount@project.com)';
        break;
      case 'Globs':
        placeHolderText =
          'Add globs, one per line (e.g. *@google.com, project:project-prefix-*)';
        break;
      case 'Subgroups':
        placeHolderText =
          'Add subgroups, one per line (e.g. administrators, mdb/chrome-troopers, google/committers@chromium.org)';
        break;
      default:
        placeHolderText = 'Add new members, one per line';
        break;
    }

    const handleRemoveDialogClose = () => {
      setRemoveDialogVisible(false);
    };

    useImperativeHandle(ref, () => ({
      getItems: () => {
        return asString(items);
      },
      changeItems: (newValues: string[]) => {
        if (!valuesEqual(newValues, savedValues)) {
          setItems(asItems(newValues.sort()));
          setSavedValues(newValues);
        }
      },
      resetToSavedValues: () => {
        setItems(asItems(savedValues));
      },
    }));

    const resetTextfield = () => {
      setAddingItem(!addingItem);
      setNewItems('');
      setInvalidValueError('');
      setDuplicateValueError('');
    };

    const addToItems = () => {
      if (validateItems()) {
        const updatedItems = [...items];
        let newItemsArray = newItems
          .split(/[\n ]+/)
          .filter((item) => item !== '');
        if (name === 'Members' || name === 'Globs') {
          newItemsArray = addPrefixToItems('user', newItemsArray);
        }
        updatedItems.push(...asItems(newItemsArray));
        setItems(updatedItems);
        resetTextfield();
      }
    };

    const validateItems = useCallback(() => {
      // Make sure item added is not a duplicate.
      let newItemsArray = newItems
        .split(/[\n ]+/)
        .filter((item) => item !== '');
      // Ensure members and globs have the user prefix added before duplicate
      // validation, as it is implied if no prefix is specified.
      // e.g. `user:*@example.com` is a duplicate of `*example.com`.
      if (name === 'Members' || name === 'Globs') {
        newItemsArray = addPrefixToItems('user', newItemsArray);
      }
      const hasValues = (value: string) => {
        for (const item of items) {
          if (name === 'Members') {
            if (item.value.toLowerCase() === value.toLowerCase()) {
              return true;
            }
          } else {
            if (item.value === value) {
              return true;
            }
          }
        }
        return false;
      };
      const duplicateValues = newItemsArray.filter((value) => hasValues(value));
      let isValid: (item: string) => boolean;
      switch (name) {
        case 'Members':
          isValid = isMember;
          break;
        case 'Globs':
          isValid = isGlob;
          break;
        case 'Subgroups':
          isValid = isSubgroup;
          break;
        default:
          isValid = () => {
            return false;
          };
      }
      const invalidValues = newItemsArray.filter((value) => !isValid(value));

      // Set error messages.
      setInvalidValueError(
        stripPrefixFromItems(userPrefix, invalidValues).join(', '),
      );
      setDuplicateValueError(
        stripPrefixFromItems(userPrefix, duplicateValues).join(', '),
      );
      return invalidValues.length === 0 && duplicateValues.length === 0;
    }, [items, name, newItems]);

    useEffect(() => {
      // If any items have been added or removed, we submit.
      // This ensure we do not submit values on initial load & on checkbox polarity change.
      if (!valuesEqual(asString(items), savedValues)) {
        submitValues();
      }
      // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [items, savedValues]);

    const handleChange = (index: number) => {
      const updatedItems = [...items];
      // This is a new item, so just remove.
      if (index >= savedValues.length) {
        updatedItems.splice(index, 1);
      } else {
        updatedItems[index].checked = !updatedItems[index].checked;
      }
      setItems(updatedItems);
    };

    // If there are any checked items. (Used to know if remove button should be visible).
    const hasSelected = () => {
      for (const item of items) {
        if (item.checked) {
          return true;
        }
      }
      return false;
    };

    // Removes checked items from items list.
    const removeItems = () => {
      const updatedItems = [...items];
      for (let i = updatedItems.length - 1; i >= 0; i--) {
        if (updatedItems[i].checked) {
          updatedItems.splice(i, 1);
        }
      }
      setItems(updatedItems);
      setRemoveDialogVisible(false);
    };

    // Converts removed items into string for confirmation dialog.
    const getRemovedMembers = () => {
      const removedItems = [];
      for (const item of items) {
        if (item.checked) {
          removedItems.push(item.value);
        }
      }
      return removedItems.join(', ');
    };

    const valuesEqual = (a: string[], b: string[]) => {
      return a.length === b.length && a.every((val, index) => val === b[index]);
    };

    useEffect(() => {
      validateItems();
    }, [newItems, validateItems]);

    const isValid = !invalidValueError && !duplicateValueError;

    return (
      <TableContainer data-testid="groups-form-list">
        <Table sx={{ width: '100%' }} data-testid="mouse-enter-table">
          <TableBody>
            <TableRow>
              <TableCell
                colSpan={2}
                style={{
                  alignItems: 'center',
                  minHeight: '40px',
                }}
              >
                <Typography variant="h6">
                  {`${name} (${items.length})`}
                </Typography>
                {items.length > expansionThreshold && (
                  <IconButton
                    onClick={() => {
                      setExpanded(!expanded);
                    }}
                  >
                    {expanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
                  </IconButton>
                )}
                {hasSelected() && (
                  <Button
                    variant="contained"
                    color="error"
                    sx={{ ml: 2 }}
                    startIcon={<RemoveCircleOutlineIcon />}
                    onClick={() => setRemoveDialogVisible(true)}
                    data-testid="remove-button"
                  >
                    Remove
                  </Button>
                )}
              </TableCell>
            </TableRow>
            {(expanded || items.length <= expansionThreshold) && (
              <>
                {items &&
                  items.map((item, index) => (
                    <TableRow
                      key={index}
                      style={{ height: '34px' }}
                      sx={{ borderBottom: '1px solid rgb(224, 224, 224)' }}
                      className="item-row"
                      data-testid={`item-row-${item.value}`}
                      role="listitem"
                    >
                      <TableCell
                        sx={{ p: 0, pt: '1px' }}
                        style={{
                          display: 'flex',
                          flexDirection: 'row',
                          alignItems: 'center',
                          minHeight: '30px',
                        }}
                      >
                        <Checkbox
                          sx={{ pt: 0, pb: 0 }}
                          checked={item.checked}
                          data-testid={`checkbox-button-${item.value}`}
                          id={`${index}`}
                          onChange={() => {
                            handleChange(index);
                          }}
                        />
                        {name === 'Subgroups' ? (
                          <GroupLink name={item.value} />
                        ) : (
                          <AuthLookupLink principal={item.value} />
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
              </>
            )}
            {addingItem && (
              <>
                <TableRow>
                  <TableCell sx={{ pt: '8px' }}>
                    <TextField
                      multiline
                      placeholder={placeHolderText}
                      label={placeHolderText}
                      style={{ width: '100%' }}
                      onChange={(e) => setNewItems(e.target.value)}
                      value={newItems}
                      data-testid="add-textfield"
                      error={!isValid}
                      helperText={
                        <span>
                          {invalidValueError !== '' && (
                            <>
                              {`Invalid ${name}: ${invalidValueError}`}
                              {duplicateValueError !== '' && <br />}
                            </>
                          )}
                          {duplicateValueError !== '' && (
                            <>{`Duplicate ${name}: ${duplicateValueError}`}</>
                          )}
                        </span>
                      }
                    ></TextField>
                  </TableCell>
                </TableRow>
                <TableRow>
                  <TableCell sx={{ pt: '8px', pb: '16px' }}>
                    {isValid && newItems !== '' && (
                      <Button
                        sx={{ mr: 1.5 }}
                        variant="contained"
                        color="success"
                        onClick={() => {
                          addToItems();
                        }}
                        data-testid="confirm-button"
                      >
                        Confirm
                      </Button>
                    )}
                    <Button
                      variant="contained"
                      color="error"
                      onClick={resetTextfield}
                      data-testid="clear-button"
                    >
                      Cancel
                    </Button>
                  </TableCell>
                </TableRow>
              </>
            )}
            {!addingItem && (
              <TableRow>
                <TableCell sx={{ pt: '8px', pb: '16px' }}>
                  <Button
                    variant="outlined"
                    startIcon={<AddCircleIcon />}
                    onClick={resetTextfield}
                    data-testid="add-button"
                  >
                    Add
                  </Button>
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
        <Dialog
          open={removeDialogVisible || false}
          onClose={handleRemoveDialogClose}
          data-testid="remove-confirm-dialog"
        >
          <DialogTitle>
            {`Are you sure you want to remove the following ${name.toLowerCase()}?`}
          </DialogTitle>
          <DialogContent>
            <DialogContentText>{getRemovedMembers()}</DialogContentText>
          </DialogContent>
          <DialogActions>
            <Button
              onClick={handleRemoveDialogClose}
              disableElevation
              variant="outlined"
            >
              Cancel
            </Button>
            <Button
              onClick={removeItems}
              disableElevation
              variant="contained"
              color="error"
              data-testid="remove-confirm-button"
            >
              Remove
            </Button>
          </DialogActions>
        </Dialog>
      </TableContainer>
    );
  },
);
GroupFormList.displayName = 'GroupFormList';
