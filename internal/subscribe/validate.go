package subscribe

import (
	"crypto/ecdh"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/gfanton/gnotif/internal/pushhost"
)

const (
	maxEndpoint = 1000
	maxTarget   = 256
	p256dhSize  = 65
	authSize    = 16
)

// checkEndpoint accepts an https URL on a push service of hosts, as
// pushhost.Match reads them.
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
	if _, ok := pushhost.Match(raw, hosts); !ok {
		return fmt.Errorf("push service %s is not allowed", u.Host)
	}
	return nil
}

func checkKey(name, value string, size int) error {
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(value, "="))
	if err != nil || len(b) != size {
		return fmt.Errorf("%s must be %d bytes in base64url", name, size)
	}
	if name == "p256dh" {
		if _, err := ecdh.P256().NewPublicKey(b); err != nil {
			return errors.New("p256dh must be a P-256 public key")
		}
	}
	return nil
}
