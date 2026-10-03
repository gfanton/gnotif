// Package trigger models gnotif triggers, the realm events they watch, and
// the notifications they render.
package trigger

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

// Pair is one key and value, as in a realm event's attributes.
type Pair struct{ Key, Value string }

// Event is a GnoEvent from a successful transaction.
type Event struct {
	TxHash  string
	Height  int64
	TxIndex int
	Index   int // position in the transaction's events array, counting every kind
	PkgPath string
	Type    string
	Attrs   []Pair
}

// Attr returns the value of the first attribute named key.
func (e Event) Attr(key string) (string, bool) {
	for _, p := range e.Attrs {
		if p.Key == key {
			return p.Value, true
		}
	}
	return "", false
}

// Trigger is a rule declared in the gnotif registry realm.
type Trigger struct {
	ID, Target, Event, Param, Title, Body, Link string
	Filter                                      []Pair
	Declarer                                    string
	Verified                                    bool
}

// Notification is what a browser shows for one matched event.
type Notification struct{ Title, Body, Link string }

// Byte limits of a rendered notification. A longer link is replaced by "/".
const (
	MaxTitle = 64
	MaxBody  = 255
	MaxLink  = 1024
)

// ErrMalformed reports a TriggerDeclared event that does not describe a trigger.
var ErrMalformed = errors.New("malformed trigger event")

var declaredKeys = [...]string{"id", "target", "event", "filter", "param", "title", "body", "link", "declarer", "verified"}

// ParseFilter reads comma-separated key=value pairs. An empty string has no pairs.
func ParseFilter(s string) ([]Pair, error) {
	if s == "" {
		return nil, nil
	}
	var out []Pair
	for item := range strings.SplitSeq(s, ",") {
		key, value, ok := strings.Cut(item, "=")
		if !ok || key == "" || value == "" || strings.Contains(value, "=") {
			return nil, fmt.Errorf("invalid filter pair %q", item)
		}
		out = append(out, Pair{Key: key, Value: value})
	}
	return out, nil
}

// FormatFilter is the inverse of ParseFilter.
func FormatFilter(pairs []Pair) string {
	items := make([]string, len(pairs))
	for i, p := range pairs {
		items[i] = p.Key + "=" + p.Value
	}
	return strings.Join(items, ",")
}

// FromDeclared builds a trigger from the attributes of a TriggerDeclared event.
func FromDeclared(attrs []Pair) (Trigger, error) {
	e := Event{Attrs: attrs}
	v := make(map[string]string, len(declaredKeys))
	for _, key := range declaredKeys {
		value, ok := e.Attr(key)
		if !ok {
			return Trigger{}, fmt.Errorf("%w: missing %s", ErrMalformed, key)
		}
		v[key] = value
	}
	for _, key := range []string{"id", "target", "event", "title", "link", "declarer"} {
		if v[key] == "" {
			return Trigger{}, fmt.Errorf("%w: empty %s", ErrMalformed, key)
		}
	}
	filter, err := ParseFilter(v["filter"])
	if err != nil {
		return Trigger{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	var verified bool
	switch v["verified"] {
	case "true":
		verified = true
	case "false":
	default:
		return Trigger{}, fmt.Errorf("%w: verified %q", ErrMalformed, v["verified"])
	}
	return Trigger{
		ID:       v["id"],
		Target:   v["target"],
		Event:    v["event"],
		Param:    v["param"],
		Title:    v["title"],
		Body:     v["body"],
		Link:     v["link"],
		Filter:   filter,
		Declarer: v["declarer"],
		Verified: verified,
	}, nil
}

// Matches reports whether e comes from the trigger's realm, has its type and
// carries every filter pair. The subscriber's parameter value is not checked.
func (t Trigger) Matches(e Event) bool {
	if e.PkgPath != t.Target || e.Type != t.Event {
		return false
	}
	for _, f := range t.Filter {
		if v, ok := e.Attr(f.Key); !ok || v != f.Value {
			return false
		}
	}
	return true
}

// Render fills the trigger's templates with e's attributes.
func (t Trigger) Render(e Event) Notification {
	link := expand(t.Link, e, url.PathEscape)
	if len(link) > MaxLink {
		link = "/"
	}
	return Notification{
		Title: truncate(expand(t.Title, e, nil), MaxTitle),
		Body:  truncate(expand(t.Body, e, nil), MaxBody),
		Link:  link,
	}
}

// expand replaces each {key} in tmpl with e's attribute key, escaped when
// escape is set. A "{" without a closing "}" is copied as is.
func expand(tmpl string, e Event, escape func(string) string) string {
	var b strings.Builder
	for {
		before, after, ok := strings.Cut(tmpl, "{")
		if !ok {
			break
		}
		name, rest, ok := strings.Cut(after, "}")
		if !ok {
			break
		}
		b.WriteString(before)
		value, _ := e.Attr(name)
		if escape != nil {
			value = escape(value)
		}
		b.WriteString(value)
		tmpl = rest
	}
	b.WriteString(tmpl)
	return b.String()
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
