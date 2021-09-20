// Copyright 2021 The LUCI Authors.
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

////////////////////////////////////////////////////////////////////////////////
// Utility functions.

// Trims group description to fit single line.
const trimGroupDescription = (desc) => {
  'use strict';
  if (desc == null) {
    return '';
  }
  var firstLine = desc.split('\n')[0];
  if (firstLine.length > 55) {
    firstLine = firstLine.slice(0, 55) + '...';
  }
  return firstLine;
}

////////////////////////////////////////////////////////////////////////////////
// GroupChooser is a scrollable list containing the auth service groups.
class GroupChooser {

  constructor(element) {
    // Root DOM element.
    this.element = document.getElementById(element);
  }

  // Loads list of groups from a server.
  // Updates group chooser UI. Returns deferred.
  refetchGroups() {
    var self = this;
    var defer = api.groups();
    defer
      .then((response) => {
        self.setGroupList(response.groups);
      })
      .catch((err) => {
        console.log(err);
      });
    return defer;
  }

  // Sets groupList (group-chooser) element.
  setGroupList(groups) {
    // Adds list item to group-chooser.
    const addElement = (group) => {
      if ('content' in document.createElement('template')) {
        var template = document.querySelector('#group-scroller-row-template');

        // Clone and grab elements to modify.
        var clone = template.content.cloneNode(true);
        var listEl = clone.querySelector('li');
        var name = clone.querySelector('p');
        var description = clone.querySelector('small');

        // Modify contents and append to parent.
        listEl.setAttribute('data-group-name', group.name);
        name.textContent = group.name;
        description.textContent = trimGroupDescription(group.description);
        this.element.appendChild(clone);
      } else {
        // TODO(cjacomet): Find another way to add group-chooser items.
        // HTML template element is not supported.
        console.error('Unable to load HTML template element, not supported.')
      }
      listEl.addEventListener('click', () => {
        this.setSelection(group.name);
      });
    };

    groups.map((group) => {
      addElement(group);
    });
  }

  // Adds the active class to the selected element,
  // highlighting the group clicked in the scroller.
  setSelection(name) {
    let selectionMade = false;
    const selectionChangedEvent = new CustomEvent('selectionChanged', {
      bubble: true,
      detail: {
        group: name,
      }
    });
    const groupElements = Array.from(document.getElementsByClassName('list-group-item'));

    groupElements.forEach((currentGroup) => {
      if (currentGroup.dataset.groupName === name) {
        currentGroup.classList.add('active');
      } else {
        currentGroup.classList.remove('active');
      }
    });
    this.element.dispatchEvent(selectionChangedEvent);
  }

  // Selects first group available.
  selectDefault() {
    let elements = document.getElementsByClassName('list-group-item');
    if (elements.length) {
      this.setSelection(elements[0].dataset.groupName);
    }
  }

}

////////////////////////////////////////////////////////////////////////////////
// Common code for 'New group' and 'Edit group' forms.
class GroupForm {

  constructor(element, groupName, template) {
    // The content containing the form and heading.
    this.element = document.querySelector(element);
    // Name of the group this form operates on.
    this.groupName = groupName;
    // Template to use: (new or edit).
    this.template = document.querySelector(template);
  }

}

////////////////////////////////////////////////////////////////////////////////
// Form to view/edit existing groups.
class EditGroupForm extends GroupForm {

  constructor(groupName) {
    // Call parent constructor.
    super('#group-content', groupName, '#edit-group-form-template');
    // Name of the group this form operates on.
    this.groupName = groupName;
  }

  // Get group response and build the form.
  load() {
    var defer = api.groupRead(this.groupName);
    defer.then((response) => {
      this.buildForm(response);
    });
    return defer;
  }

  // Prepare response for html text content.
  buildForm(group) {
    const groupClone = { ...group };

    var members = (groupClone.members ? groupClone.members.map((member) => common.stripPrefix('user', member)) : []);
    var globs = (groupClone.globs ? groupClone.globs.map((glob) => common.stripPrefix('user', glob)) : []);
    var membersAndGlobs = [].concat(members, globs);

    // TODO(cjacomet): Assert that membersAndGlobs can be split.

    // Convert lists into a single text blob.
    groupClone.membersAndGlobs = membersAndGlobs.join('\n') + '\n';
    groupClone.nested = (groupClone.nested || []).join('\n') + '\n';

    // TODO(cjacomet): Set up external group handling.
    // TODO(cjacomet): Set up form submission and deletion.
    this.populateForm(groupClone);
  }

  // Populates the form with the text lists of the group.
  populateForm(group) {
    if ('content' in document.createElement('template')) {
      // Clone and grab elements to modify.
      var clone = this.template.content.cloneNode(true);
      var heading = clone.querySelector('#group-heading');
      var description = clone.querySelector('#description-box');
      var owners = clone.querySelector('#owners-box');
      var membersAndGlobs = clone.querySelector('#membersAndGlobs');
      var nested = clone.querySelector('#nested');

      // Clear any previous html.
      this.element.innerHTML = '';

      // Modify contents and append to parent.
      heading.textContent = group.name;
      description.textContent = group.description;
      owners.textContent = group.owners;
      membersAndGlobs.textContent = group.membersAndGlobs
      nested.textContent = group.nested;
      this.element.appendChild(clone);
    } else {
      // TODO(cjacomet): Find another way to add group-content group.
      // HTML template element is not supported.
      console.error('Unable to load HTML template element, not supported.')
    }
  }

}

window.onload = () => {
  // Setup global UI elements.
  var groupChooser = new GroupChooser('group-chooser');

  const startEditGroupFlow = (groupName) => {
    let form = new EditGroupForm(groupName);
    form.load();
  };

  groupChooser.element.addEventListener('selectionChanged', (event) => {
    if (event.detail.group === null) {
      console.log('new group flow');
    } else {
      startEditGroupFlow(event.detail.group);
    }
  });

  const jumpToCurrentGroup = (selectDefault) => {
    if (selectDefault) {
      groupChooser.selectDefault();
    }
  };

  groupChooser.refetchGroups().then(() => {
    jumpToCurrentGroup(true);
  });
};