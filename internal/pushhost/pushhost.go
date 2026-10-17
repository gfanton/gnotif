// Package pushhost matches push endpoints against the allowlist of push
// services.
package pushhost

import (
	"net/url"
	"strings"
)

// Match returns the entry of hosts that endpoint's URL is on. An entry
// "*.suffix" matches any host under suffix on the default port; any other
// entry must equal the URL's host, port included. Case is ignored. ok is
// false when no entry matches or endpoint is not a URL.
func Match(endpoint string, hosts []string) (entry string, ok bool) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", false
	}
	for _, entry := range hosts {
		if suffix, ok := strings.CutPrefix(entry, "*."); ok {
			if u.Port() == "" && strings.HasSuffix(strings.ToLower(u.Hostname()), "."+strings.ToLower(suffix)) {
				return entry, true
			}
			continue
		}
		if strings.EqualFold(u.Host, entry) {
			return entry, true
		}
	}
	return "", false
}
