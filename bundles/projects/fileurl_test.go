package projects

import (
	"strings"
	"testing"
	"time"
)

func TestFileTokenRoundTrip(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	tok := signFileToken("nate", "my world", "0123456789abcdef0123456789abcdef01234567", now.Add(time.Hour))
	owner, name, rev, err := verifyFileToken(tok, now)
	if err != nil || owner != "nate" || name != "my world" || !strings.HasPrefix(rev, "01234567") {
		t.Fatalf("round trip: %q %q %q %v", owner, name, rev, err)
	}
	if _, _, _, err := verifyFileToken(tok, now.Add(2*time.Hour)); err != ErrBadFileToken {
		t.Errorf("expired token accepted: %v", err)
	}
	payload, sig, _ := strings.Cut(tok, ".")
	forged := signFileToken("someone-else", "my world", "0123456789abcdef0123456789abcdef01234567", now.Add(time.Hour))
	forgedPayload, _, _ := strings.Cut(forged, ".")
	for _, bad := range []string{"", "x", payload, payload + ".", forgedPayload + "." + sig, payload + "." + sig[:len(sig)-2] + "AA"} {
		if _, _, _, err := verifyFileToken(bad, now); err != ErrBadFileToken {
			t.Errorf("%q accepted: %v", bad, err)
		}
	}
}
