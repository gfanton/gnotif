package gnotif

import "testing"

func mustPanic(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		t.Helper()
		got := recover()
		if got == nil {
			t.Fatalf("no panic, want %q", want)
		}
		if got != want {
			t.Fatalf("panic %v, want %q", got, want)
		}
	}()
	f()
}

func TestPackFields(t *testing.T) {
	got := packFields("ab", "", "c:d")
	if want := "2:ab0:3:c:d"; got != want {
		t.Fatalf("packFields = %q, want %q", got, want)
	}
}

func TestUnpackFieldsRoundTrip(t *testing.T) {
	in := []string{"gno.land/r/alice/chat/v0", "Message", "room=1:2", "", "Title: 42", "{a}:{b}", "/x"}
	got := unpackFields(packFields(in...), len(in))
	if len(got) != len(in) {
		t.Fatalf("got %d fields, want %d", len(got), len(in))
	}
	for i := range in {
		if got[i] != in[i] {
			t.Errorf("field %d = %q, want %q", i, got[i], in[i])
		}
	}
}

func TestRecordRoundTrip(t *testing.T) {
	r := record{
		Target: "gno.land/r/alice/chat/v0",
		T: Trigger{
			Event:  "Message",
			Filter: "k=v",
			Param:  "to",
			Title:  "New: message",
			Body:   "{text}",
			Link:   "/",
		},
	}
	if got := decodeRecord(r.encode()); got != r {
		t.Fatalf("decodeRecord(encode) = %+v, want %+v", got, r)
	}
}

func TestUnpackFieldsCorrupt(t *testing.T) {
	cases := []struct{ name, in string }{
		{"length past the end", "3:ab"},
		{"length not a number", "x:abc"},
		{"bytes left over", "2:ab1:c1:d"},
		{"missing field", "2:ab"},
		{"negative length", "-1:a"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mustPanic(t, "corrupt record", func() { unpackFields(c.in, 2) })
		})
	}
}
