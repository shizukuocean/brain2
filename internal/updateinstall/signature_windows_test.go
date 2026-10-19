//go:build windows

package updateinstall

import "testing"

func TestPublisherMatchRequiresWholeSubjectOrExactCommonName(t *testing.T) {
	for _, test := range []struct {
		subject  string
		expected string
		match    bool
	}{
		{subject: "CN=Meow Software, O=Meow Studio, C=CN", expected: "Meow Software", match: true},
		{subject: "CN=Meow Software, O=Meow Studio, C=CN", expected: "CN=Meow Software", match: true},
		{subject: "CN=Meow Software, O=Meow Studio, C=CN", expected: "Meow", match: false},
		{subject: "CN=Other Meow Software, O=Meow Studio", expected: "Meow Software", match: false},
	} {
		if got := publisherMatches(test.subject, test.expected); got != test.match {
			t.Errorf("publisherMatches(%q, %q)=%v", test.subject, test.expected, got)
		}
	}
}
