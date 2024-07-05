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

////////////////////////////////////////////////////////////////////////////////
// Component to display the services listing.
class ServicesTable {
    constructor(data) {
        this.element = document.querySelector('#services-table-template').content.cloneNode(true);
        this.replicaRowTemplate = document.querySelector('#replica-service-row-template');

        this.primaryServiceId = this.element.querySelector('#primary-service-id');
        this.primaryAuthCodeVersion = this.element.querySelector('#primary-auth-code-version');
        this.tableBody = this.element.querySelector('tbody');
        this.lastUpdatedLabel = this.element.querySelector('#last-updated');

        this.setData(data);
    }

    setData(data) {
        // Add the primary service's details.
        this.primaryServiceId.textContent = data.primaryRevision.primaryId;
        this.primaryAuthCodeVersion.textContent = data.authCodeVersion;

        // Add a row for each replica service.
        data.replicas.forEach((service) => {
            const replicaRow = this.replicaRowTemplate.content.cloneNode(true);

            // Link to the replica service.
            const serviceID = replicaRow.querySelector('#service-id');
            serviceID.textContent = service.appId;
            serviceID.setAttribute('href', service.baseUrl);

            // Set the replica's status depending on the AuthDB revision and
            // status of the last push.
            let pushStatusDetails = { message: 'Unknown', classes: ['bg-secondary'] };
            if (service.authDbRev === data.primaryRevision.authDbRev) {
                // Replica's AuthDB is up to date.
                pushStatusDetails = { message: 'OK', classes: ['bg-success'] };
            } else if (service.pushStatus === 'SUCCESS' || service.pushStatus === 'TRANSIENT_ERROR') {
                // The replica's AuthDB is stale, but we expect the next push to
                // succeed.
                pushStatusDetails = { message: 'Syncing', classes: ['bg-warning', 'text-dark'] };
            } else if (service.pushStatus === 'FATAL_ERROR') {
                // The replica's AuthDB is stale, and the most recent push
                // failed fatally.
                pushStatusDetails = { message: 'Error', classes: ['bg-danger'] };
            }
            const statusBadge = replicaRow.querySelector('#push-status');
            statusBadge.textContent = pushStatusDetails.message;
            statusBadge.classList.add(...pushStatusDetails.classes);

            // Set the replica's AuthDB revision and auth code version.
            replicaRow.querySelector('#auth-code-version').textContent = service.authCodeVersion;

            // Calculate the lag (the duration of the last push attempt,
            // successful or not).
            if (Object.hasOwn(service, 'pushStarted')
                && Object.hasOwn(service, 'pushFinished')) {
                const lag = Date.parse(service.pushFinished) - Date.parse(service.pushStarted);
                replicaRow.querySelector('#lag').textContent = `${lag}`;
            }

            this.tableBody.appendChild(replicaRow);
        });

        this.lastUpdatedLabel.textContent = common.utcTimestampToString(data.processedAt);
    }
}


////////////////////////////////////////////////////////////////////////////////
// Component to trigger refreshing.
class RefreshBtn {
    constructor(element, refreshFn) {
        this.element = document.querySelector(element);
        this.label = this.element.querySelector('span');
        this.disable();

        // Call the given refresh function when this button is clicked.
        this.element.addEventListener('click', () => {
            this.disable();
            refreshFn().finally(() => {
                this.enable();
            });
        })
    }

    enable() {
        this.element.disabled = false;
        this.label.textContent = 'Refresh';
    }

    disable() {
        this.element.disabled = true;
        this.label.textContent = 'Refreshing...';
    }
}


window.onload = () => {
    const loadingBox = new common.LoadingBox('#loading-box-placeholder');
    const servicesContent = new common.HidableElement('#services-content', false);
    const servicesTableContainer = document.querySelector('#services-table-container');
    const errorBox = new common.ErrorBox('#api-error-placeholder');

    const updateServiceListing = () => {
        errorBox.clearError();

        return api.listReplicas()
        .then((response) => {
            const servicesTable = new ServicesTable(response);
            servicesTableContainer.innerHTML = '';
            servicesTableContainer.appendChild(servicesTable.element);
        })
        .catch((err) => {
            errorBox.showError("Fetching info for replicas failed", err.error);
        });
    };

    // Set the refresh button to update the service listing data.
    const refreshBtn = new RefreshBtn('#refresh-btn', updateServiceListing);

    // Do the initial page load.
    loadingBox.setLoadStatus(true);
    updateServiceListing().then(() => {
        loadingBox.setLoadStatus(false);
        refreshBtn.enable();
        servicesContent.show();
    });
}
