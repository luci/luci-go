import { LitElement, html, css, customElement, property, PropertyValues } from 'lit-element';
import { repeat } from 'lit-html/directives/repeat';

import { TemplateResult, render } from 'lit-html';
import { styleMap } from 'lit-html/directives/style-map';


interface ListUpdatedEventDetail {
    items: string[];
}

export type ListUpdatedEvent = CustomEvent<ListUpdatedEventDetail>;

@customElement('tr-sortable-list')
export class SortableList extends LitElement {
    private draggingItemKey: string | null = null;
    private items: [string, TemplateResult][] = [];

    private itemKeyTemplateResultMap: Map<string, TemplateResult> = new Map();
    private currentItemKeys: string[] = [];
    private sortedItemKeys: string[] = [];


    private positions: { [key: string]: { x: number, y: number } | undefined } = {};
    private oldPositions: { [key: string]: { x: number, y: number } | undefined } = {};

    private staticLayer?: HTMLElement = undefined;

    private getStaticLayerItemKey(itemKey: string) {
        return 'static:' + itemKey;
    }

    private getDynamicLayerItemKey(itemKey: string) {
        return 'dynamic:' + itemKey;
    }

    protected shouldUpdate(changes: PropertyValues) {
        if (changes.has('items')) {
            this.itemKeyTemplateResultMap = new Map(this.items);
            this.currentItemKeys = this.items.map(([key]) => key);
            this.sortedItemKeys = this.currentItemKeys.slice().sort();

            render(this.items.map(([key, template]) => html`
            <div draggable="true" slot=${this.getDynamicLayerItemKey(key)}>${template}</div>
            <div draggable="true" slot=${this.getStaticLayerItemKey(key)}>${template}</div>
            `), this);
        }

        return true;
    }

    protected firstUpdated() {
        this.staticLayer = this.shadowRoot?.querySelector('.layer.static')! as HTMLElement;
    }

    protected updated() {
        const parentPos = this.shadowRoot?.querySelector('.layer.static')
            ?.getBoundingClientRect()!;

        let updated = false;
        this.oldPositions = this.positions;
        this.positions = {};

        this.shadowRoot?.querySelectorAll('.layer.static>*').forEach((ele) => {
            const childPos = ele.getBoundingClientRect();
            const newChildRelPos = { x: childPos.x - parentPos.x, y: childPos.y - parentPos.y };
            const oldChildRelPos = this.oldPositions[ele.id];
            if (newChildRelPos.x !== oldChildRelPos?.x || newChildRelPos.y !== oldChildRelPos?.y) {
                updated = true;
            }
            this.positions[ele.id] = newChildRelPos;
        });
        if (updated) {
            this.requestUpdate();
        }
    }

    protected render() {
        return html`
        <div class="layer dynamic">
            ${repeat(this.sortedItemKeys, (key) => key, (key) => html`
            <div
                @dragstart=${() => {
                    this.draggingItemKey = key;
                    setTimeout(() => this.staticLayer!.style.visibility = 'visible');
                }}
                @dragend=${() => {
                    this.dispatchEvent(new CustomEvent<ListUpdatedEventDetail>('list-updated', {
                        detail: { items: this.currentItemKeys.map((key) => key) },
                        bubbles: false,
                        composed: true,
                    }));
                    this.draggingItemKey = null;
                    this.staticLayer!.style.visibility = 'hidden';
                }}
                class="drag-container"
                style=${styleMap({
                    transform: `translate(${this.positions[key]?.x || 0}px, ${this.positions[key]?.y || 0}px)`,
                    transition: this.oldPositions[key] ? '0.5s' : '0s',
                })}
            >
                <slot name=${this.getDynamicLayerItemKey(key)}></slot>
            </div>
            `)}
        </div>
        <div class="layer static">
            ${repeat(this.currentItemKeys, (key) => key, (key) => html`
            <div
                id=${key}
                @dragenter=${() => {
                    if (this.draggingItemKey === key) {
                        return;
                    }
                    const i = this.currentItemKeys.indexOf(this.draggingItemKey!);
                    const j = this.currentItemKeys.indexOf(key);
                    // mutating instead of recreating because this function will be called repeatedly
                    this.currentItemKeys.splice(i, 1);
                    this.currentItemKeys.splice(j + (i < j ? 1 : 0), 0, this.draggingItemKey!);
                    // manually request update because this.movingItems are mutated instead of being assigned a new value
                    this.requestUpdate('currentItemKeys');
                }}
                class="drag-container"
            >
                <slot name=${this.getStaticLayerItemKey(key)}></slot>
            </div>
            `)}
        </div>
        `;
    }

    static styles = css`
        :host {
            display: grid;
        }
        :host > * {
            grid-area: 1/1;
        }
        .layer {
            margin-top: 0px;
            margin-left: 0px;
        }
        .layer.static {
            visibility: hidden;
            opacity: 0;
        }
        .drag-container {
            overflow: auto;
        }
        .layer.dynamic {
            position: relative;
        }
        .layer.dynamic > .drag-container {
            position: absolute;
        }
        `;
}
