/**
 * Extend URL with methods that can be chained.
 */
export class ChainableURL extends URL {
  withSearchParam(key: string, value: string, override = false) {
    if (override) {
      this.searchParams.set(key, value);
    } else {
      this.searchParams.append(key, value);
    }
    return this;
  }
}

// Generates URL for collecting feedback.
export function genFeedbackUrl() {
  const feedbackComment =
    `From Link: ${document.location.href}\n` +
    'Please enter a description of the problem, with repro steps if applicable.';

  const searchParams = new URLSearchParams({
    template: 'Build Infrastructure',
    components: 'Infra>Platform>Milo>ResultUI',
    labels: 'Pri-2,Type-Bug',
    comment: feedbackComment,
  });
  return `https://bugs.chromium.org/p/chromium/issues/entry?${searchParams}`;
}

/**
 * Hash the message with SHA-256 and then return the outcome in hex encoded
 * string.
 */
export async function sha256(message: string) {
  const msgBuffer = new TextEncoder().encode(message);
  const hashBuffer = await crypto.subtle.digest('SHA-256', msgBuffer);
  const hashArray = Array.from(new Uint8Array(hashBuffer));
  const hashHex = hashArray.map((b) => b.toString(16).padStart(2, '0')).join('');
  return hashHex;
}

/**
 * Returns a promise that resolves after the specified duration.
 */
export function timeout(ms: number) {
  return new Promise<void>((resolve) => setTimeout(resolve, ms));
}

/**
 * A utility function that helps set additional search query params.
 */
export function urlSetSearchQueryParam(url: string, key: string, value: { toString(): string }) {
  const urlObj = new URL(url);
  urlObj.searchParams.set(key, value.toString());
  return urlObj.toString();
}

/**
 * Convert a value to an Error if it's not an Error.
 */
export function toError(from: unknown): Error {
  return from instanceof Error ? from : new Error(`${from}`);
}
