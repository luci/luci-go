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
  let firstLine = desc.split('\n')[0];
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
    this.element = document.querySelector(element);

    // Button for triggering create group workflow.
    this.createGroupBtn = document.querySelector("#create-group-btn");
  }

  // Loads list of groups from a server.
  // Updates group chooser UI. Returns deferred.
  refetchGroups() {
    const self = this;
    return api.groups()
      .then((response) => {
        self.setGroupList(response.groups);
      })
      .catch((err) => {
        console.log(err);
      });
  }

  // Sets groupList (group-chooser) element.
  setGroupList(groups) {
    // Adds list item to group-chooser.
    const addElement = (group) => {
      const template = document.querySelector('#group-scroller-row-template');

      // Clone and grab elements to modify.
      const clone = template.content.cloneNode(true);
      const listEl = clone.querySelector('li');
      const name = clone.querySelector('p');
      const description = clone.querySelector('small');

      // Modify contents and append to parent.
      listEl.setAttribute('data-group-name', group.name);
      name.textContent = group.name;
      description.textContent = trimGroupDescription(group.description);
      this.element.appendChild(clone);

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
// ContentFrame is the outer frame that will load a GroupForm, this can be
// a NewGroupForm or an EditGroupForm.
class ContentFrame {
  constructor(elementSelector) {
    // The root element of this ContentFrame (the container for the GroupForm).
    this.element = document.querySelector(elementSelector);
    // The content that this frame currently has loaded.
    // This will be an instance of GroupForm class.
    this.content = null;
    // What the frame is currently trying to load, used to check if another loadContent
    // call was made before loading is done (i.e. user clicks a different group).
    this.loading = null;
  }

  // Sets the content of the content frame.
  // Clears any content that was previously in the frame.
  setContent(content) {
    // Empty the dom element.
    this.element.innerHTML = '';

    // Set to the new content.
    this.content = content;
    this.loading = null;

    if (this.content) {
      this.element.appendChild(this.content.element);
      // TODO(cjacomet): Trigger content shown handler.
    }
  }

  // Loads new content asynchronously using content.load(...) call.
  // |content| is an instance of GroupForm class.
  loadContent(content) {
    let self = this;
    // TODO(cjacomet): Disable interaction while we load content.
    self.loading = content;
    return content.load()
      .then(() => {
        // Switch content only if another 'loadContent' wasn't called before.
        if (self.loading == content) {
          self.setContent(content);
        }
      })
      .catch((error) => {
        // Still loading same content?
        if (self.loading == content) {
          self.setContent(null);
          // TODO(cjacomet): Load error pane or alert instead of console.log.
          console.log("error unable to load content");
        }
      });
  };
}

////////////////////////////////////////////////////////////////////////////////
// Common code for 'New group' and 'Edit group' forms.
class GroupForm {

  constructor(templateName, groupName) {
    // The cloned template of the respective form we'll be loading.
    this.element = document.querySelector(templateName).content.cloneNode(true);
    // Name of the group this form operates on.
    this.groupName = groupName;
  }

  // returns a resolved promise, the subclass should override this when
  // making an RPC call.
  load() {
    return new Promise(resolve => {
      resolve();
    });
  }


}

////////////////////////////////////////////////////////////////////////////////
// Form to view/edit existing groups.
class EditGroupForm extends GroupForm {

  constructor(groupName) {
    // Call parent constructor.
    super('#edit-group-form-template', groupName);
  }

  // Get group response and build the form.
  load() {
    return api.groupRead(this.groupName).then((response) => {
      this.buildForm(response);
    });
  }

  // Prepare response for html text content.
  buildForm(group) {
    const groupClone = { ...group };

    const members = (groupClone.members ? groupClone.members.map((member) => common.stripPrefix('user', member)) : []);
    const globs = (groupClone.globs ? groupClone.globs.map((glob) => common.stripPrefix('user', glob)) : []);
    const membersAndGlobs = [].concat(members, globs);

    // TODO(cjacomet): Assert that membersAndGlobs can be split.

    // Convert lists into a single text blob.
    groupClone.membersAndGlobs = membersAndGlobs.join('\n') + '\n';
    groupClone.nested = (groupClone.nested || []).join('\n') + '\n';

    // TODO(cjacomet): Set up external group handling.
    this.populateForm(groupClone);
  }

  // Populates the form with the text lists of the group.
  populateForm(group) {
    // Grab form fields.
    const heading = this.element.querySelector('#group-heading');
    const description = this.element.querySelector('#description-box');
    const owners = this.element.querySelector('#owners-box');
    const membersAndGlobs = this.element.querySelector('#membersAndGlobs');
    const nested = this.element.querySelector('#nested');
    const deleteBtn = this.element.querySelector('#delete-btn');

    // Modify contents.
    heading.textContent = group.name;
    description.textContent = group.description;
    owners.textContent = group.owners;
    membersAndGlobs.textContent = group.membersAndGlobs
    nested.textContent = group.nested;


    deleteBtn.addEventListener('click', () => {
      let result = confirm(`Are you sure you want to delete ${group.name}?`)
      if (result) {
        console.log(`attempting to delete ${group.name}...`);
        api.groupDelete(group.name, group.etag)
          .then(() => {
            console.log(`deleted ${group.name} successfully!`);
            // TODO(cjacomet): Optimize this to just reload content of scroller and not entire window.
            location.reload();
          })
          .catch((error) => {
            console.log(`failed trying to delete ${group.name}: ${error}`);
            // TODO(cjacomet): Replace alert with error modal to display error to users through UI.
            alert(`failed trying to delete ${group.name}: ${error}`);
          });
      }
    })
  }
}

////////////////////////////////////////////////////////////////////////////////
// Form to create a new group.
class NewGroupForm extends GroupForm {
  constructor() {
    super('#new-group-form-template', '');
  }
}

window.onload = () => {
  // Setup global UI elements.
  const groupChooser = new GroupChooser('#group-chooser');
  const contentFrame = new ContentFrame('#group-content');

  const startNewGroupFlow = () => {
    let form = new NewGroupForm();
    contentFrame.loadContent(form);
  };

  const startEditGroupFlow = (groupName) => {
    let form = new EditGroupForm(groupName);
    contentFrame.loadContent(form);
  };

  groupChooser.element.addEventListener('selectionChanged', (event) => {
    if (event.detail.group === null) {
      console.log('new group flow');
    } else {
      startEditGroupFlow(event.detail.group);
    }
  });

  groupChooser.createGroupBtn.addEventListener('click', (event) => {
    startNewGroupFlow();
    groupChooser.setSelection(null);
  })

  const jumpToCurrentGroup = (selectDefault) => {
    if (selectDefault) {
      groupChooser.selectDefault();
    }
  };

  groupChooser.refetchGroups().then(() => {
    jumpToCurrentGroup(true);
  });
};