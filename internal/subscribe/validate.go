package subscribe

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/gfanton/gnotif/internal/trigger"
)

const (
	maxEndpoint = 1000
	maxOptins   = 50
	maxValue    = 4096
	p256dhSize  = 65
	authSize    = 16
)

// checkEndpoint accepts an https URL on an allowlisted push service. An
// entry "*.suffix" matches any host under suffix on the default port; any
// other entry must equal the URL's host, port included.
func checkEndpoint(raw string, hosts []string) error {
	if len(raw) > maxEndpoint {
		return fmt.Errorf("endpoint longer than %d bytes", maxEndpoint)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return errors.New("endpoint must be an https URL")
	}
	if u.User != nil {
		return errors.New("endpoint must not carry user information")
	}
	for _, entry := range hosts {
		if suffix, ok := strings.CutPrefix(entry, "*."); ok {
			if u.Port() == "" && strings.HasSuffix(strings.ToLower(u.Hostname()), "."+strings.ToLower(suffix)) {
				return nil
			}
			continue
		}
		if strings.EqualFold(u.Host, entry) {
			return nil
		}
	}
	return fmt.Errorf("push service %s is not allowed", u.Host)
}

func checkKey(name, value string, size int) error {
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(value, "="))
	if err != nil || len(b) != size {
		return fmt.Errorf("%s must be %d bytes in base64url", name, size)
	}
	return nil
}

func checkOptins(optins []optinJSON, triggers map[string]trigger.Trigger) error {
	if len(optins) > maxOptins {
		return fmt.Errorf("more than %d opt-ins", maxOptins)
	}
	for _, o := range optins {
		t, ok := triggers[o.Trigger]
		switch {
		case !ok:
			return fmt.Errorf("unknown trigger %q", o.Trigger)
		case t.Param == "" && o.Value != "":
			return fmt.Errorf("trigger %s takes no value", t.ID)
		case t.Param != "" && o.Value == "":
			return fmt.Errorf("trigger %s needs a value for %s", t.ID, t.Param)
		case len(o.Value) > maxValue:
			return fmt.Errorf("value longer than %d bytes", maxValue)
		case o.Value != strings.TrimSpace(o.Value):
			// Event attributes never carry surrounding spaces, so such a
			// value would be stored and never match.
			return errors.New("value has leading or trailing white space")
		}
	}
	return nil
}
