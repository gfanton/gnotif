// safeLink resolves link against the service worker's scope, and gives the
// scope for a link that lands on another origin, such as "//host" or "/\host".
export function safeLink(link: string, scope: string): string {
  try {
    const url = new URL(link, scope);
    if (url.origin === new URL(scope).origin) {
      return url.href;
    }
  } catch {
    // an unparsable link gives the scope
  }
  return scope;
}
