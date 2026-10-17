package pushhost

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatch(t *testing.T) {
	hosts := []string{"fcm.googleapis.com", "*.notify.windows.com", "127.0.0.1:8443"}
	cases := map[string]struct {
		endpoint string
		entry    string
		ok       bool
	}{
		"exact host":               {"https://fcm.googleapis.com/fcm/send/abc", "fcm.googleapis.com", true},
		"host in another case":     {"https://FCM.googleapis.com/fcm/send/abc", "fcm.googleapis.com", true},
		"wildcard":                 {"https://wns2-par02p.notify.windows.com/w/?token=x", "*.notify.windows.com", true},
		"wildcard in another case": {"https://WNS2-par02p.Notify.Windows.com/w/", "*.notify.windows.com", true},
		"exact host and port":      {"https://127.0.0.1:8443/push", "127.0.0.1:8443", true},
		"unlisted host":            {"https://push.evil.example/x", "", false},
		"unlisted port":            {"https://fcm.googleapis.com:8443/x", "", false},
		"wildcard with a port":     {"https://wns2-par02p.notify.windows.com:8443/w/", "", false},
		"bare wildcard suffix":     {"https://notify.windows.com/x", "", false},
		"suffix without a dot":     {"https://evilnotify.windows.com/x", "", false},
		"userinfo trick":           {"https://fcm.googleapis.com@evil.example/x", "", false},
		"not a URL":                {"https://fcm.googleapis.com/%zz", "", false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			entry, ok := Match(tc.endpoint, hosts)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.entry, entry)
		})
	}
}
