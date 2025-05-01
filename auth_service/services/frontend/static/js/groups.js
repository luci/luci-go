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
// Constants.

const GROUP_ROOT_URL = "/auth/groups/";
const NEW_GROUP_PLACEHOLDER = "new!";

////////////////////////////////////////////////////////////////////////////////
// Utility functions.

// True if group name starts with '<something>/' prefix, where
// <something> is a non-empty string.
function isExternalGroupName(name) {
  return name.indexOf('/') > 0;
}

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

// Given a set of strings, returns the first one with longest
// substring match with the search term.
const longestMatch = (items, text) => {
  if (text.length === 0 || items.size === 0) {
    return null;
  }

  // Invariant: curSet is non empty subsequence of 'items';
  // each item in curSet has 'curPrefix' as a substring.
  var curPrefix = '';
  var curSet = items;
  for (var i = 0; i < text.length; i++) {
    // Attempt to increase curPrefix.
    var newPrefix = curPrefix + text[i];
    var newSet = new Set();
    curSet.forEach((item) => {
      if (item.indexOf(newPrefix) != -1) {
        newSet.add(item);
      }
    });
    // No matches at all -> curSet contains longest matches.
    if (newSet.size === 0) {
      // Could not find the first letter -> no match at all.
      if (i === 0) {
        return null;
      }
      return curSet.values().next().value;
    }

    // Carry on.
    curPrefix = newPrefix;
    curSet = newSet;
  }
  // curSet is a subset of 'items' that have 'text' as substring, pick first.
  return curSet.values().next().value;
}

////////////////////////////////////////////////////////////////////////////////
// Address bar manipulation.

const getCurrentGroupInURL = () => {
  let p = window.location.pathname;
  if (p.startsWith(GROUP_ROOT_URL)) {
    return decodeURIComponent(p.slice(GROUP_ROOT_URL.length));
  }
  return '';
}

const setCurrentGroupInURL = (groupName) => {
  if (getCurrentGroupInURL() != groupName) {
    window.history.pushState({ 'group': groupName }, null,
      common.getGroupPageURL(groupName));
  }
}

const onCurrentGroupInURLChange = (cb) => {
  window.onpopstate = function (event) {
    let s = event.state;
    if (s && s.hasOwnProperty('group')) {
      cb(s.group);
    };
  };
}

////////////////////////////////////////////////////////////////////////////////
// Form Validation.

// Form validation regex.
const groupsRe = /^([a-z\-]+\/)?[0-9a-z_\-\.@]{1,100}$/;
const membersRe = /^((user|bot|project|service|anonymous)\:)?[\w\-\+\%\.\@\*\[\]]+$/;

// Splits 'value' on lines boundaries, trims spaces and returns lines
// as an array of strings.
const splitItemList = (list) => {
  return list.split('\n').map((item) => item.trim()).filter((item) => !!item);
};

// True if string looks like a glob pattern (and not a group member name).
const isGlob = (item) => {
  // Glob patterns contain '*' and '[]' not allowed in member names.
  return item.search(/[\*\[\]]/) != -1;
};

// Sets classes for invalid element.
const setInvalid = (formObj, errorMsg) => {
  formObj.element.classList.add('is-invalid');
  formObj.errorElement.textContent = errorMsg;
  formObj.errorElement.className = "error active text-danger";
}

// Resets the validity of the field's html classes.
const resetValidity = (fieldElement, errorElement) => {
  fieldElement.classList.remove('is-invalid');
  errorElement.textContent = "";
  errorElement.className = "error";
}

// Set of callbacks and respective error message for each type of validation.
// Key comes from class names added to HTML form-control element inside the
// current form.
const validators = {
  'groupName': [
    (value) => { return groupsRe.test(value); },
    'Invalid group name.'
  ],
  'groupNameOrEmpty': [
    (value) => { return !value || groupsRe.test(value); },
    'Invalid group name.'
  ],
  'membersAndGlobsList': [
    (value) => {
      return splitItemList(value).every((item) => membersRe.test(item));
    },
  ],
  'groupList': [
    (value) => {
      return splitItemList(value).every((item) => groupsRe.test(item));
    },
    'Invalid group name.'
  ],
  'required': [
    (value) => { return !!value },
    'Field is required.',
  ],
};

////////////////////////////////////////////////////////////////////////////////
// ListGroupItem is a list item representing a single group. It is
// intended to be used within a GroupChooser.
class ListGroupItem {
  constructor(group) {
    this.readOnly = !group.callerCanModify;
    this.isExternal = isExternalGroupName(group.name);

    const template = document.querySelector('#group-scroller-row-template');

    // Clone and grab elements to modify.
    const clone = template.content.cloneNode(true);
    const rootEl = clone.querySelector('a');
    const nameEl = rootEl.querySelector('p');
    const descEl = rootEl.querySelector('small');

    // Modify contents for the actual group name and description.
    rootEl.setAttribute('data-group-name', group.name);
    rootEl.setAttribute('href', common.getGroupPageURL(group.name));
    nameEl.textContent = group.name;
    descEl.textContent = this.isExternal ? 'External' : trimGroupDescription(group.description);
    if (this.readOnly) {
      rootEl.classList.add('read-only-group');
      const lockIcon = document.createElement('i');
      lockIcon.classList.add('bi', 'bi-lock-fill', 'list-item-icon');
      rootEl.appendChild(lockIcon);
    }

    this.element = rootEl;
  }

  setExternalVisibility(showExternal) {
    if (this.isExternal) {
      this.element.style.display = showExternal ? 'block' : 'none';
    }
  }
}

////////////////////////////////////////////////////////////////////////////////
// GroupChooser is a scrollable list containing the auth service groups.
class GroupChooser {

  constructor(element) {
    // Root DOM element.
    this.element = document.querySelector(element);

    // The name of the selected group.
    this.selectedGroupName = null;

    // Whether the group selected can be changed.
    this.allowChange = true;

    // Sets of known groups, used for checking group presence and
    // selecting a default group.
    this.completeGroupSet = new Set();
    this.internalOnlyGroupSet = new Set();

    // Mapping of group name -> ListGroupItem.
    this.groupOptions = new Map();

    // Checkbox for showing external groups.
    this.showExternalCheckBox = document.querySelector("#external-check");
    this.showExternalCheckBox.addEventListener("input", () => {
      this.setExternalVisibility();

      // If the currently selected group is no longer visible, change the
      // selected group.
      if (this.reselectionRequired()) {
        this.selectDefault();
      }
    });
  }

  // getGroupSet is a helper to return the appropriate group set
  // depending on the external checkbox.
  getGroupSet() {
    if (this.showExternalCheckBox.checked) {
      return this.completeGroupSet;
    }
    return this.internalOnlyGroupSet;
  }

  reselectionRequired() {
    return this.selectedGroupName && !this.isKnownGroup(this.selectedGroupName);
  }

  // Checks if set of known groups contains queried group.
  isKnownGroup(groupName) {
    return this.getGroupSet().has(groupName);
  }

  refetchGroups(requireFresh) {
    return api.groups(requireFresh).then((response) => {
      this.setGroupList(response.groups);
    });
  }

  // Sets groupList (group-chooser) element.
  setGroupList(groups) {
    // Adds list item to group-chooser.
    const addElement = (group) => {
      const listItem = new ListGroupItem(group);

      // Add it to the group chooser.
      this.element.appendChild(listItem.element);

      // Add a listener for when a user chooses the group.
      listItem.element.onclick = () => {
        if (this.allowChange) {
          this.setSelection(group.name);
        }
        return false;
      };

      // Keep a reference to the ListGroupItem, so we can scroll to it
      // later.
      this.groupOptions.set(group.name, listItem);

      // Maintain the group name sets, with and without external groups.
      this.completeGroupSet.add(group.name);
      if (!listItem.isExternal) {
        this.internalOnlyGroupSet.add(group.name);
      }
    };

    // Clear the previous group list data.
    this.element.innerHTML = '';
    this.selectedGroupName = null;
    this.completeGroupSet = new Set();
    this.internalOnlyGroupSet = new Set();
    this.groupOptions = new Map();

    groups.map((group) => {
      addElement(group);
    });

    this.setExternalVisibility();
  }

  // Adds the active class to the selected element,
  // highlighting the group clicked in the scroller.
  // Optional: success message to be added to the selectionChanged event; this
  // is useful when providing feedback on user actions after form submission.
  setSelection(name, message = '') {
    if (this.selectedGroupName === name) {
      return;
    }

    this.selectedGroupName = name;
    let selectionMade = false;
    const selectionChangedEvent = new CustomEvent('selectionChanged', {
      bubble: true,
      detail: {
        group: name,
        success: message,
      }
    });
    const groupElements = Array.from(document.getElementsByClassName('list-group-item'));

    groupElements.forEach((currentGroup) => {
      if (currentGroup.dataset.groupName === name) {
        selectionMade = true;
        currentGroup.classList.add('active');
      } else {
        currentGroup.classList.remove('active');
      }
    });

    if (selectionMade) {
      this.ensureGroupVisible(name);
      this.element.dispatchEvent(selectionChangedEvent);
    }
  }

  // Selects first group visible.
  selectDefault() {
    const groupSet = this.getGroupSet();
    if (groupSet.size) {
      this.setSelection(groupSet.values().next().value);
    }
  }

  // Scrolls group list so that the given group is visible.
  ensureGroupVisible(name) {
    let groupOption = this.groupOptions.get(name);
    if (!groupOption) {
      return;
    }

    let chooserRect = this.element.getBoundingClientRect();
    let optionRect = groupOption.element.getBoundingClientRect();

    // Scroll to the selected group if it's not completely visible.
    if ((optionRect.top < chooserRect.top) || (optionRect.bottom > chooserRect.bottom)) {
      this.element.scrollTop += optionRect.top - chooserRect.top;
    }
  };

  // setExternalVisibility sets the visibility of each group option in
  // the chooser depending on whether external groups should be shown.
  setExternalVisibility() {
    this.groupOptions.forEach((listGroupItem, _) => {
      listGroupItem.setExternalVisibility(this.showExternalCheckBox.checked);
    });
  }

  enableInteraction() {
    this.allowChange = true;
  }

  disableInteraction() {
    this.allowChange = false;
  }
}

////////////////////////////////////////////////////////////////////////////////
// Text field to search for a group.

class SearchBox {

  constructor(element) {
    // Root DOM element.
    this.element = document.querySelector(element);
  }

}

////////////////////////////////////////////////////////////////////////////////
// ContentFrame is the outer frame that will load a GroupForm, this can be
// a NewGroupForm or an EditGroupForm.
class ContentFrame {
  constructor(elementSelector) {
    const container = document.querySelector(elementSelector);
    // The root element of this ContentFrame (the container for the GroupForm).
    this.element = container.querySelector('#content-form');
    // The element to display a message if there's an error loading content.
    this.errorBox = new common.ErrorBox('#content-load-error-placeholder');
    // The content that this frame currently has loaded.
    // This will be an instance of GroupForm class.
    this.content = null;
    // What the frame is currently trying to load, used to check if another loadContent
    // call was made before loading is done (i.e. user clicks a different group).
    this.loading = null;
  }

  // Registers a new event listener to be called when content is loaded.
  addContentLoadedListener(cb) {
    this.element.addEventListener('contentLoaded', () => {
      cb();
    });
  }

  // Sets the content of the content frame.
  // Clears any content that was previously in the frame.
  setContent(content) {
    // Dispose of any tooltips from the old content.
    if (this.content instanceof EditGroupForm) {
      this.content.clearTooltips();
    }

    // Empty the element containing the GroupForm.
    this.element.innerHTML = '';

    // Set to the new content.
    this.content = content;
    this.loading = null;

    if (this.content) {
      this.element.appendChild(this.content.element);
    }
  }

  // Loads new content asynchronously using content.load(...) call.
  // |content| is an instance of GroupForm class.
  loadContent(content) {
    // Clear the previous error message, if any.
    this.errorBox.clearError();

    // Disable interaction of the old content while loading the new content.
    if (this.content) {
      this.content.setInteractionDisabled(true);
    }

    this.loading = content;
    return content.load()
      .then(() => {
        // Switch content only if another 'loadContent' wasn't called before.
        if (this.loading == content) {
          this.setContent(content);
        }
      })
      .catch((err) => {
        // Still loading same content?
        if (this.loading == content) {
          this.setContent(null);
          this.errorBox.showError('Failed loading group details', err.error);
        }
      })
      .finally(() => {
        if (content.groupName) {
          setCurrentGroupInURL(content.groupName);
        }
        this.element.dispatchEvent(new CustomEvent('contentLoaded'));
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

    // The Form element.
    this.form = this.element.querySelector('#group-form');

    // The spinner to show while the form is being submitted.
    this.spinner = this.element.querySelector('#submit-spinner');

    // The element for displaying form submission feedback.
    this.feedback = this.element.querySelector('#submit-feedback');
    this.feedbackTemplate = document.querySelector('#submit-feedback-template');

    // Is the form valid to submit?
    this.valid = false;

    // Field elements of the form, we rely on the 'form-control' class. If another
    // field is added to the form it must have 'form-control' in the class to
    // be picked up for validation.
    const fieldElems = Array.from(this.form.getElementsByClassName('form-control'));

    // Maintains a list of the fields respective event listeners and validation
    // function(s) necessary to validate a given field.
    this.fields = fieldElems.map((element) => {

      // Attach event listener to this field.
      const validatorNames = Array.from(element.classList).filter((name) => { return name in validators; })
      const errorElement = element.nextElementSibling;
      const formFieldObj = { element, validatorNames, errorElement };
      element.addEventListener('input', () => {
        this.validate(formFieldObj);
      });
      return formFieldObj
    });

    // Event listener to trigger validation workflow when form submit event
    // happens.
    this.form.addEventListener('submit', (event) => {
      event.preventDefault();
      this.valid = true;
      this.fields.forEach((formField) => { this.validate(formField); });
      if (this.valid) {
        const authGroup = this.createAuthGroupFromForm();
        this.doSubmit(authGroup);
      }
    })
  }

  createAuthGroupFromForm() {
    const formValues = this.fields.reduce((map, obj) => {
      map[obj.element.id] = obj.element.value.trim();
      return map;
    }, {});
    // Create group vs update group.
    const groupName = this instanceof NewGroupForm ? formValues['group-name-box'] : this.groupName;
    let membersList = [];
    let globsList = [];
    splitItemList(formValues['membersAndGlobs']).forEach((item) => { isGlob(item) ? globsList.push(item) : membersList.push(item); });
    const subGroups = splitItemList(formValues['nested']);
    const desc = formValues['description-box'];
    const ownersVal = formValues['owners-box'];

    return {
      "name": groupName,
      "members": common.addPrefixToItems('user', membersList),
      "globs": common.addPrefixToItems('user', globsList),
      "nested": subGroups,
      "description": desc,
      "owners": ownersVal,
    };
  }

  // returns a resolved promise, the subclass should override this when
  // making an RPC call.
  load() {
    return new Promise(resolve => {
      resolve();
    });
  }

  // Updates the value of the current formField Object
  // then calls validation callbacks on the given field.
  validate(formFieldObj) {
    const { element, errorElement } = formFieldObj;
    const value = element.value.trim();
    resetValidity(element, errorElement);

    formFieldObj.validatorNames.forEach((name) => {
      const isValid = validators[name][0](value);
      if (!isValid) {
        this.valid = false;
        setInvalid(formFieldObj, validators[name][1]);
      }
    })
  }

  doSubmit(authGroup) {
    // Should be overidden.
    throw new Error('doSubmit not implemented');
  }

  clearAlert() {
    // Clear the feedback section.
    this.feedback.innerHTML = '';
  }

  #showAlert(alertClass, title, message) {
    this.clearAlert();

    // Clone the feedback template and set the class, title and message.
    let fb = this.feedbackTemplate.content.cloneNode(true).querySelector('div');
    fb.classList.add(alertClass);
    fb.querySelector('strong').textContent = title;
    fb.querySelector('span').textContent = message;

    // Add the alert to the feedback section.
    this.feedback.appendChild(fb);
  }

  showSuccessAlert(title) {
    this.#showAlert('alert-success', title, '');
  }

  showErrorAlert(message) {
    this.#showAlert('alert-danger', 'Oh snap!', message);
  }

  showSpinner() {
    this.spinner.style.display = 'block';
  }

  hideSpinner() {
    this.spinner.style.display = 'none';
  }

  setInteractionDisabled(disabled) {
    // Sets the disabled attribute for all buttons, inputs and textareas in the
    // form.
    this.form.querySelectorAll('button, input, textarea').forEach((e) => {
      if (disabled) {
        e.setAttribute('disabled', '');
      } else {
        e.removeAttribute('disabled');
      }
    });
  }
}

////////////////////////////////////////////////////////////////////////////////
// BuildFormError is thrown by EditGroupForm on unsuccessful loading
// of group data. Its structure is similar to api.CallError for
// consistency of handling and displaying error messages in the UI.
class BuildFormError extends Error {
  constructor(error) {
    super('Error building form');
    this.error = error;
  }
}

////////////////////////////////////////////////////////////////////////////////
// Form to view/edit existing groups.
class EditGroupForm extends GroupForm {

  constructor(groupName) {
    // Call parent constructor.
    super('#edit-group-form-template', groupName);

    this.groupEtag = "";

    // Whether the group form should be in read-only mode.
    this.readOnly = false;

    // Whether the group is external.
    this.isExternal = false;

    // Tooltips in this form.
    this.tooltips = [];
  }

  onUpdate(authGroup) {
    // Should be overidden.
    throw new Error('onUpdate not implemented');
  }

  onDelete(authGroup) {
    // Should be overidden.
    throw new Error('onDelete not implemented');
  }

  // Dispose of all tooltips in this form.
  clearTooltips() {
    this.tooltips.forEach((t) => {
      t.dispose();
    });
    this.tooltips = [];
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

    this.groupEtag = groupClone.etag;

    const members = (groupClone.members ? common.stripPrefixFromItems('user', groupClone.members) : []);
    const globs = (groupClone.globs ? common.stripPrefixFromItems('user', groupClone.globs) : []);

    // Assert members and globs can be split apart later.
    members.forEach((m) => {
      if (isGlob(m)) {
        console.error('Invalid member (glob-like):', m);
        throw new BuildFormError('Invalid members list');
      }
    });
    globs.forEach((g) => {
      if (!isGlob(g)) {
        console.error('Invalid glob:', g);
        throw new BuildFormError('Invalid globs list');
      }
    });

    // Convert lists into a single text blob.
    const membersAndGlobs = [].concat(members, globs);
    groupClone.membersAndGlobs = membersAndGlobs.join('\n') + '\n';
    groupClone.nested = (groupClone.nested || []).join('\n') + '\n';

    if (!group.callerCanModify) {
      // Read-only UI if the caller has insufficient permissions.
      this.makeReadOnly(group);
    }
    if (isExternalGroupName(group.name)) {
      // Read-only UI for external groups.
      this.makeReadOnly(group);
      this.makeExternal();
    }

    this.populateForm(groupClone);
  }

  setInteractionDisabled(disabled) {
    if (this.readOnly) {
      return;
    }
    super.setInteractionDisabled(disabled);
  }

  makeReadOnly(group) {
    // Exit early if this has previously been called, as form elements
    // have already been changed.
    if (this.readOnly) {
      return;
    }

    // Disable any inputs.
    this.setInteractionDisabled(true);

    // Remove update/delete buttons.
    const editBtn = this.element.querySelector('#edit-btn');
    const deleteBtn = this.element.querySelector('#delete-btn');
    this.form.removeChild(editBtn);
    this.form.removeChild(deleteBtn);

    // Add explanation on how to make group changes.
    const template = document.querySelector('#insufficient-perms-template');
    const clone = template.content.cloneNode(true);
    const ownersSection = clone.querySelector('#insufficient-perms-owners-section');
    if (group.owners != group.name) {
      // The group is not self-owned, so we should link to the owners.
      ownersSection.querySelector('a').setAttribute('href', common.getGroupPageURL(group.owners));
    } else {
      clone.removeChild(ownersSection);
    }
    this.form.appendChild(clone);

    this.readOnly = true;
  }

  makeExternal() {
    // Exit early if this has previously been called, as form elements
    // have already been changed.
    if (this.isExternal) {
      return;
    }
    this.isExternal = true;

    // Remove all but the members row.
    this.form.querySelectorAll(':scope > *').forEach((e) => {
      if (!e.classList.contains('external-group-info')) {
        this.form.removeChild(e);
      }
    });

    // Enlarge the members text area.
    const membersAndGlobs = this.element.querySelector('#membersAndGlobs');
    membersAndGlobs.setAttribute('rows', 30);
  }

  // Populates the form with the text lists of the group.
  populateForm(group) {
    // Grab heading, members form field, and links.
    const heading = this.element.querySelector('#group-heading');
    const membersAndGlobs = this.element.querySelector('#membersAndGlobs');
    const changeLogLink = this.element.querySelector('#group-change-log-link');
    const lookupLink = this.element.querySelector('#group-lookup-link');
    const listingLink = this.element.querySelector('#group-listing-link');
    const membersCol = this.element.querySelector('#membersAndGlobsCol');

    // Modify group name and members.
    heading.textContent = group.name;
    // Check for privacy filter.
    if (!group.callerCanViewMembers) {
      let numRedacted = group.numRedacted || 0;
      const redactedSpan = document.createElement('label');
      redactedSpan.textContent = numRedacted + ' members redacted';
      redactedSpan.classList.add('col-form-label');
      membersCol.appendChild(redactedSpan);
      membersAndGlobs.style.display = 'none';
    } else {
      membersAndGlobs.textContent = group.membersAndGlobs;
    }

    // Modify the links for group-specific information.
    changeLogLink.setAttribute('href',
      common.getChangeLogTargetURL('AuthGroup', group.name));
    lookupLink.setAttribute('href', common.getLookupURL(group.name));
    listingLink.setAttribute('href', common.getGroupListingURL(group.name));

    // Enable tooltips for the group's links.
    this.tooltips.push(new bootstrap.Tooltip(changeLogLink));
    this.tooltips.push(new bootstrap.Tooltip(lookupLink));
    this.tooltips.push(new bootstrap.Tooltip(listingLink));

    // Exit early if the form is for an external group.
    if (this.isExternal) {
      // Add ' (external)' to the group heading.
      const externalSpan = document.createElement('span');
      externalSpan.textContent = ' (external)';
      externalSpan.style.color = 'gray';
      heading.appendChild(externalSpan);

      return;
    }

    // Grab remaining form fields.
    const description = this.element.querySelector('#description-box');
    const owners = this.element.querySelector('#owners-box');
    const nested = this.element.querySelector('#nested');

    // Modify remaining contents.
    description.textContent = group.description;
    owners.textContent = group.owners;
    nested.textContent = group.nested;

    // Exit early if the form is read-only.
    if (this.readOnly) {
      return;
    }

    const deleteBtn = this.element.querySelector('#delete-btn');
    if (group.name != 'administrators') {
      deleteBtn.addEventListener('click', () => {
        let result = confirm(`Are you sure you want to delete ${group.name}?`)
        if (result) {
          console.log(`attempting to delete ${group.name}...`);
          this.onDelete(group);
        }
      });
    } else {
      // Remove the delete button.
      this.form.removeChild(deleteBtn);
    }
  }

  doSubmit(authGroup) {
    authGroup.etag = this.groupEtag;
    return this.onUpdate(authGroup);
  }
}

////////////////////////////////////////////////////////////////////////////////
// Form to create a new group.
class NewGroupForm extends GroupForm {
  constructor() {
    super('#new-group-form-template', '');
  }

  onCreate(authGroup) {
    // Should be overidden.
    throw new Error('onCreate not implemented');
  }

  doSubmit(authGroup) {
    return this.onCreate(authGroup);
  }
}

// Wrapper around an RPC call that originated from some form.
// Locks UI while call is running, then refreshes the list of groups once it
// resolves.
const waitForResult = (cb, groupChooser, form, listErrorBox, requireFresh) => {
  let done = new Promise((resolve, reject) => {
    // Lock the group chooser while running the request.
    groupChooser.disableInteraction();

    // Lock the current form while running the request.
    // If the submission of the current form is successful, there is no need to
    // unlock it because a new form will be instantiated so that the latest
    // group details are available.
    form.setInteractionDisabled(true);
    form.showSpinner();

    // Hide previous error message (if any).
    form.clearAlert();

    cb
      .then((response) => {
        // Call succeeded. Refetch the list of groups.
        groupChooser.refetchGroups(requireFresh)
          .then(() => {
            // Groups list updated - trigger resolve.
            resolve(response);
          })
          .catch((err) => {
            // Failed to update the groups list. Show page-wide error message and
            // trigger reject.
            listErrorBox.showError('Listing groups failed', err.error);
            reject(err);
          });
      })
      .catch((err) => {
        // First, ensure the error message starts with a capital.
        const raw = err.error || '';
        const message = raw.charAt(0).toUpperCase() + raw.slice(1);

        // Show error message on the form, since it's a local error with the
        // request, and trigger reject.
        form.showErrorAlert(message);

        // Unlock the current form.
        form.setInteractionDisabled(false);
        form.hideSpinner();

        reject(err);
      })
      .finally(() => {
        // Unlock the group chooser.
        groupChooser.enableInteraction();
      });
  });

  return done;
};

window.onload = () => {
  // Setup global UI elements.
  const integratedUIAlert = new common.IntegratedUIAlert('#integrated-ui-alert-container');
  integratedUIAlert.setLink(common.INTEGRATED_UI_GROUPS_ROOT);
  const loadingBox = new common.LoadingBox('#loading-box-placeholder');
  const mainContent = new common.HidableElement('#main-content', false);
  const groupChooser = new GroupChooser('#group-chooser');
  const contentFrame = new ContentFrame('#group-content');
  const searchBox = new SearchBox('#search-box');
  const listErrorBox = new common.ErrorBox('#list-api-error-placeholder');

  const startNewGroupFlow = () => {
    let form = new NewGroupForm();

    // Called when the 'Create' button is clicked.
    form.onCreate = (group) => {
      const request = api.groupCreate(group);
      waitForResult(request, groupChooser, form, listErrorBox, true)
        .then((response) => {
          groupChooser.setSelection(response.name, 'Group created.');
          // If the creation was done in dry-run mode, the group won't exist.
          if (groupChooser.reselectionRequired()) {
            groupChooser.selectDefault();
          }
        });
    };
    contentFrame.loadContent(form);
  };

  const startEditGroupFlow = (groupName, successMessage) => {
    let form = new EditGroupForm(groupName);

    // Called when the 'Update group' button is clicked.
    form.onUpdate = (group) => {
      const request = api.groupUpdate(group);
      waitForResult(request, groupChooser, form, listErrorBox, false)
        .then((response) => {
          groupChooser.setSelection(response.name, 'Group updated.');
        });
    };

    // Called when the 'Delete group' button is clicked.
    form.onDelete = (group) => {
      const request = api.groupDelete(group.name, group.etag);
      waitForResult(request, groupChooser, form, listErrorBox, true)
        .then(() => {
          groupChooser.selectDefault();
        });
    };

    // Once the group loads, show the success message if there is one.
    contentFrame.loadContent(form).then(() => {
      // Set the success alert message if provided.
      if (successMessage) {
        form.showSuccessAlert(successMessage);
      }
    });
  };

  groupChooser.element.addEventListener('selectionChanged', (event) => {
    if (event.detail.group === null) {
      // Exit early for new group flow.
      return;
    }

    integratedUIAlert.setLink(
      common.INTEGRATED_UI_GROUPS_ROOT + "/" + event.detail.group);
    startEditGroupFlow(event.detail.group, event.detail.success);
  });

  // Button for triggering create group workflow.
  const createGroupBtn = document.querySelector("#create-group-btn");
  if (createGroupBtn) {
    createGroupBtn.addEventListener('click', (event) => {
      setCurrentGroupInURL(NEW_GROUP_PLACEHOLDER);
      integratedUIAlert.setLink(
        common.INTEGRATED_UI_GROUPS_ROOT + "/" + NEW_GROUP_PLACEHOLDER);
      startNewGroupFlow();
      groupChooser.setSelection(null);
    });
  }

  // Check the "Show external groups" checkbox if the group in the URL
  // is an external group.
  groupChooser.showExternalCheckBox.checked = isExternalGroupName(getCurrentGroupInURL());
  groupChooser.setExternalVisibility();

  const jumpToCurrentGroup = (selectDefault) => {
    let current = getCurrentGroupInURL();
    if (current == NEW_GROUP_PLACEHOLDER) {
      startNewGroupFlow();
      groupChooser.setSelection(null);
    } else if (groupChooser.isKnownGroup(current)) {
      groupChooser.setSelection(current);
    } else if (selectDefault) {
      groupChooser.selectDefault();
    }
  };

  // Allow selecting a group via search box.
  searchBox.element.addEventListener('input', () => {
    var found = longestMatch(groupChooser.getGroupSet(), searchBox.element.value);
    if (found) {
      groupChooser.setSelection(found);
    }
  });

  // Focus on group members box if "Enter" is hit.
  searchBox.element.addEventListener('keyup', (e) => {
    if (e.keyCode == 13 && contentFrame.content) {
      contentFrame.element.querySelector('#membersAndGlobs').focus();
    }
  });

  // Present the page only when the group form has been loaded.
  contentFrame.addContentLoadedListener(() => {
    loadingBox.setLoadStatus(false);
    mainContent.show();
    groupChooser.ensureGroupVisible(groupChooser.selectedGroupName);
  });

  // Show a loading spinner when first fetching all groups.
  listErrorBox.clearError();
  loadingBox.setLoadStatus(true);
  mainContent.hide();

  groupChooser.refetchGroups(false)
    .then(() => {
      jumpToCurrentGroup(true);
      onCurrentGroupInURLChange(() => {
        jumpToCurrentGroup(false);
      });
    })
    .catch((err) => {
      listErrorBox.showError('Listing groups failed', err.error);
    });
};
