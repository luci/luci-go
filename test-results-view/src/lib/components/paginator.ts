import { customElement, html, css, property } from "lit-element";
import { MobxLitElement } from "@adobe/lit-mobx";
import { observable, computed } from "mobx";

@customElement('milo-paginator')
export class Paginator extends MobxLitElement {
    @observable
    public pageSize = 5;
    @observable
    public total: number | null = null;
    public onStartIndexUpdated: (startIndex: number) => void = () => {};

    // separate startIndex & _startIndex, so setting startIndex externally won't trigger an update event
    @observable
    public startIndex = 0;
    @computed
    private get _startIndex() {
        return Math.max(Math.min(this.startIndex, this.total ?? Infinity - 1), 0);
    }
    private set _startIndex(newStartIndex: number) {
        const oldStartIndex = this._startIndex;
        this.startIndex = newStartIndex;
        if (this._startIndex !== oldStartIndex) {
            this.onStartIndexUpdated(this._startIndex);
        }
    }

    @computed
    private get endIndex() {
        return Math.min(this._startIndex + this.pageSize, this.total ?? Infinity)
    }

    protected render() {
        return html`
        <mwc-icon-button
            @click=${() => this._startIndex = 0}
            ?disabled=${this._startIndex === 0}
            class="pagination-button"
            icon="skip_previous"
        ></mwc-icon-button>
        <mwc-icon-button
            @click=${() => this._startIndex -= this.pageSize}
            ?disabled=${this._startIndex === 0}
            class="pagination-button"
            icon="navigate_before"
        ></mwc-icon-button>
        ${this._startIndex + 1} - ${this.endIndex} / ${this.total ?? 'UNKNOWN'}
        <mwc-icon-button
            @click=${() => this._startIndex += this.pageSize}
            ?disabled=${this.endIndex === this.total}
            class="pagination-button"
            icon="navigate_next"
        ></mwc-icon-button>
        <mwc-icon-button
            @click=${() => this._startIndex = this.total! - this.pageSize}
            ?disabled=${this.total === null || this.endIndex === this.total}
            class="pagination-button"
            icon="skip_next"
        ></mwc-icon-button>
        `;
    }

    static styles = css`
        .pagination-button {
            --mdc-icon-button-size: 1em;
            --mdc-icon-size: 1em;
            vertical-align: middle;
        }
        `;
}
