export function* splitAt(str: string, indices: number[]): Generator<string> {
    let start = 0;
    for (const i of indices) {
        yield str.slice(start, i);
        start = i;
    }
}

export function* indexOfAnyChar(str: string, chars: string): Generator<number> {
    for (let i = 0; i < str.length; ++i) {
        if (chars.includes(str[i])) {
            yield i;
        }
    }
}
