/* eslint-disable @typescript-eslint/indent */
// Copyright 2022 The LUCI Authors.
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

import './elements/project_card';

import {
    css,
    customElement,
    html,
    LitElement,
    state
} from 'lit-element';

import {
    getProjectsService,
    ListProjectsRequest,
    Project
} from '../../services/project';

/**
 *  Represents the home page where the user selects their project.
 */
@customElement('home-page')
export class HomePage extends LitElement {

    @state()
    projects: Project[] | null = [];

    connectedCallback() {
        super.connectedCallback();
        this.fetch();
    }

    async fetch() {
        const service = getProjectsService();
        const request: ListProjectsRequest = {};
        const response = await service.list(request);
        // Chromium milestone projects are explicitly ignored by the backend, match this in the frontend.
        this.projects = response.projects?.filter(p => !/^(chromium|chrome)-m[0-9]+$/.test(p.project)) || null;
        this.requestUpdate();
    }

    render() {
        return html`
        <main id="container">
            <section id="title">
                <h1>Projects</h1>
            </section>
            <section id="projects">
                ${this.projects?.map((project) => {
                    return html`
                    <project-card .project=${project}></project-card>
                    `;
                })}
            </section>
        </main>
        `;
    }

    static styles = css`
    #container {
        margin-left: 16rem;
        margin-right: 16rem;
    }

    #projects {
        margin: auto;
        display: grid;
        grid-template-columns: repeat(6, 1fr);
        gap: 2rem;
    }
    `;

}
