import { LitElement, html, customElement, css } from 'lit-element';
import { MDCMenu } from '@material/menu';
import * as mdcListStyle from '@material/list/dist/mdc.list.css';
import * as mdcMenuStyle from '@material/menu/dist/mdc.menu.css';
import * as mdcMenuSurfaceStyle from '@material/menu-surface/dist/mdc.menu-surface.css';

@customElement('tr-cl-menu')
export class CLMenu extends LitElement {
    private menu?: MDCMenu;

    public open(e: MouseEvent) {
        this.menu!.open = true;
        this.menu!.setAbsolutePosition(e.x, e.y);
    }

    public close() {
        this.menu!.open = false;
    }

    protected render() {
        return html`
        <div class="mdc-menu mdc-menu-surface">
            <ul class="mdc-list" role="menu" aria-hidden="true" aria-orientation="vertical" tabindex="-1">
                <li class="mdc-list-item" role="menuitem">
                    <span class="mdc-list-item__text">Open blame list</span>
                </li>
            </ul>
        </div>
        `;
    }

    static styles = css``;

    protected firstUpdated() {
        this.menu = new MDCMenu(this.shadowRoot!.querySelector('.mdc-menu')!);
    }

    constructor() {
        super();
        (this.shadowRoot as any).adoptedStyleSheets = [
            mdcListStyle, mdcMenuStyle, mdcMenuSurfaceStyle,
            ...(this.shadowRoot as any).adoptedStyleSheets,
        ];
    }
}
