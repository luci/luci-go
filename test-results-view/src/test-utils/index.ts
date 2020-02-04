
import { action } from 'mobx';

const sleep = (ms: number) => new Promise((resolve) => setTimeout(() => resolve(), ms));

class TestUtils {
    constructor() {
        this.init();
    }

    @action
    async init() {
        await this.initTestInvocationStore();
    }

    @action
    async initTestInvocationStore() {
    }
}

const testUtils = new TestUtils();

(window as any).testUtils = testUtils;
