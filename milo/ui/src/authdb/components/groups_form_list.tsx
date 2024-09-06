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
import Button from '@mui/material/Button';
import IconButton from '@mui/material/IconButton';
import ClearIcon from '@mui/icons-material/Clear';
import CancelIcon from '@mui/icons-material/Cancel';
import AddCircleIcon from '@mui/icons-material/AddCircle';
import DoneIcon from '@mui/icons-material/Done';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableRow from '@mui/material/TableRow';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import {useState, forwardRef, useImperativeHandle, useEffect} from 'react';
import { isGlob, isMember, isSubgroup } from '@/authdb/common/helpers';

import './groups_list.css';

interface GroupsFormListProps {
    // Sets the starting items array. Used on initial GetGroup call from groups_form.
    initialItems: string[];
    // This will be either members, subgroups or globs. Used for header in form and to check validity of added items.
    name: string;
    // groups_form function called when items are added or removed in this array.
    // Also called when items are 'reset' (same as initial).
    // This allows groups_form component to know of edited state.
    itemsChanged: () => void;
}

export interface FormListElement {
  getItems: () => string[];
  setReadonly: () => void;
  changeItems: (items: string[]) => void;
  isChanged: () => boolean;
}

export const GroupsFormList = forwardRef<FormListElement, GroupsFormListProps>(
  (
  {initialItems, name, itemsChanged}, ref
  ) => {
    const [items, setItems] = useState<string[]>(initialItems);
    const [addingItem, setAddingItem] = useState<boolean>();
    const [currentItem, setCurrentItem] = useState<string>();
    const [errorMessage, setErrorMessage] = useState<string>('');
    const [removedItems, setRemovedItems] = useState<string[]>();
    const [removedMessage, setRemovedMessage] = useState<string>('');

    useImperativeHandle(ref, () => ({
      getItems: () => items,
      setReadonly: () => {
        setAddingItem(false);
        setCurrentItem("");
      },
      changeItems: (items: string[]) => {
        setItems(items);
        setRemovedItems([]);
      },
      isChanged: () => {
        return !(items.length === initialItems.length && items.every((val, index) => val === initialItems[index]));
      }
    }));

    const resetTextfield = () => {
        setAddingItem(!addingItem);
        setCurrentItem('');
        setErrorMessage('');
      }

    const addToItems = () => {
      // Make sure item added is not a duplicate.
      if (items.includes(currentItem!)) {
        setErrorMessage('Duplicate item.');
        return;
      }
      // If this is members or globs, verify accordingly before adding.
      // If it doesn't meet the requirements, show error message.
      if (name == 'Members') {
        if (!isMember(currentItem!)) {
          setErrorMessage('Each member should be an email address.');
          return;
        }
      } else if (name == 'Globs') {
        if (!isGlob(currentItem!)) {
          setErrorMessage('Each glob should use at least one wildcard (i.e. *).');
          return;
        }
      } else if (name == 'Subgroups') {
        if (!isSubgroup(currentItem!)) {
          setErrorMessage('Invalid subgroup name.');
          return;
        }
      }
      if (currentItem) {
        // If we re-add a previously removed member, delete it from removed items array.
        if (removedItems) {
          let newRemovedItems = removedItems.filter(item => item !== currentItem);
          setRemovedItems(newRemovedItems);
        }
        setItems([...items, currentItem]);
        resetTextfield()
      }
    }

    useEffect(() => {
      // Every time removed items array is updated, also update message shown.
      updateRemovedMessage();
    }, [removedItems])

    useEffect(() => {
      itemsChanged();
    }, [items])

    const isNewItem = (item: string) => {
      if (initialItems.includes(item)) {
        return false;
      }
      return true;
    }

    const updateRemovedMessage = () => {
      if (removedItems && removedItems.length > 0) {
        let newRemovedMessage = 'Removed: ' + removedItems.join(', ');
        setRemovedMessage(newRemovedMessage);
      } else {
        setRemovedMessage('');
      }
    }

    const removeFromItems = (index: number) => {
        // We only consider if initial items are removed.
        if (!isNewItem(items[index])) {
          setRemovedItems([...removedItems || [], items[index]]);
          updateRemovedMessage();
        }
        setItems(items.filter((_, i) => i != index));
    }

    const submitItem = (e: React.KeyboardEvent<HTMLDivElement>) => {
        if (e.key == 'Enter') {
          addToItems();
        }
      }

    return (
    <TableContainer data-testid='groups-form-list'>
    <Table sx={{ p: 0, pt: '15px', width: '100%' }} data-testid='mouse-enter-table'>
        <TableBody>
      <TableRow>
        <TableCell colSpan={2} sx={{pb :0}} style={{display: 'flex', flexDirection: 'row', alignItems:'center', minHeight: '45px'}}>
          <Typography variant="h6"> {name}</Typography>
        </TableCell>
      </TableRow>
      {items && items.map((item, index) =>
        <TableRow key={index} style={{height: '34px'}} sx={{borderBottom: '1px solid grey'}} className='item-row' data-testid={`item-row-${item}`}>
          <TableCell sx={{p: 0, pt: '1px'}} style={{display: 'flex', flexDirection: 'row', alignItems:'center', minHeight: '30px'}}>
              <IconButton className='remove-icon' color='error' sx={{p: 0, ml: 0.5, mr: 0.5}} onClick={() => removeFromItems(index as number)} data-testid={`remove-button-${item}`}>
                <CancelIcon/>
              </IconButton>
            {isNewItem(item)
            ? <Typography variant="body2" color="green">{item}</Typography>
            : <Typography variant="body2">{item}</Typography>
            }
          </TableCell>
        </TableRow>
      )}
      {addingItem && (
        <TableRow>
          <TableCell sx={{p: 0, pt: '15px', pr: '15px'}} style={{width: '94%'}}>
            <TextField label='Add New' style={{width: '100%'}} onChange={(e) => setCurrentItem(e.target.value)} onKeyDown={submitItem} value={currentItem} data-testid='add-textfield' error={errorMessage !== ''} helperText={errorMessage}></TextField>
          </TableCell>
          <TableCell align='center' style={{width: '3%'}} sx={{p: 0, pt: '15px'}}>
            <IconButton color='success' sx={{p: 0}} onClick={() => {addToItems()}} data-testid='confirm-button'>
              <DoneIcon />
            </IconButton>
          </TableCell>
          <TableCell align='center' style={{width: '3%'}} sx={{p: 0, pt: '15px'}}>
            <IconButton color='error' sx={{p: 0}} onClick={resetTextfield} data-testid='clear-button'>
              <ClearIcon />
            </IconButton>
          </TableCell>
        </TableRow>)
      }
      {!addingItem &&
        <TableRow>
            <TableCell>
          <Button sx={{mt: '10px'}} variant="outlined" startIcon={<AddCircleIcon />} onClick={resetTextfield} data-testid='add-button'>
            Add
          </Button>
          </TableCell>
      </TableRow>
      }
      {(removedItems && removedItems.length > 0) &&
        <TableRow>
          <TableCell sx={{pt: 0, pb: 0}}>
            <Typography variant="subtitle1" sx={{p: 0}}>{removedMessage}</Typography>
          </TableCell>
        </TableRow>
      }
      </TableBody>
    </Table>
  </TableContainer>);
}
);