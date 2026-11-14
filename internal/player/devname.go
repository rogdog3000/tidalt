package player

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// asoundRoot is where card ids are read from; overridden in tests.
var asoundRoot = "/proc/asound"

// ALSA card numbers are assigned in probe order and change across reboots and
// replugs (a USB DAC that was card 5 can come back as card 0, with another
// device taking 5). Persisting "hw:5,0" therefore silently redirects audio to
// whatever card holds that number next boot. The device is instead persisted
// by the card's stable id ("hw:CARD=Amplifier,DEV=0") and resolved back to the
// current number whenever it is opened.

// StableDeviceName converts "hw:N,D" / "plughw:N,D" into the card-id form
// "hw:CARD=<id>,DEV=D" for persistence. Strings it cannot convert (already
// stable, "default", unknown card) are returned unchanged.
func StableDeviceName(device string) string {
	prefix, rest, ok := splitALSAPrefix(device)
	if !ok {
		return device
	}
	var card, dev int
	if _, err := fmt.Sscanf(rest, "%d,%d", &card, &dev); err != nil {
		return device
	}
	id := cardID(card)
	if id == "" {
		return device
	}
	return fmt.Sprintf("%s:CARD=%s,DEV=%d", prefix, id, dev)
}

// ResolveDevice converts the card-id form "hw:CARD=<id>,DEV=D" back into the
// numeric "hw:N,D" for the card's current number. Numeric strings and anything
// unrecognised are returned unchanged; an id with no matching card (DAC
// unplugged) is also returned unchanged so opening it fails loudly rather than
// playing through some other card.
func ResolveDevice(device string) string {
	prefix, rest, ok := splitALSAPrefix(device)
	if !ok || !strings.HasPrefix(rest, "CARD=") {
		return device
	}
	id, devPart, _ := strings.Cut(strings.TrimPrefix(rest, "CARD="), ",")
	dev := "0"
	if v, found := strings.CutPrefix(devPart, "DEV="); found && v != "" {
		dev = v
	}
	n, found := cardNumForID(id)
	if !found {
		return device
	}
	return fmt.Sprintf("%s:%d,%s", prefix, n, dev)
}

func splitALSAPrefix(device string) (prefix, rest string, ok bool) {
	for _, p := range []string{"hw", "plughw"} {
		if r, found := strings.CutPrefix(device, p+":"); found {
			return p, r, true
		}
	}
	return "", "", false
}

func cardID(card int) string {
	b, err := os.ReadFile(filepath.Join(asoundRoot, fmt.Sprintf("card%d", card), "id"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func cardNumForID(id string) (int, bool) {
	matches, _ := filepath.Glob(filepath.Join(asoundRoot, "card[0-9]*", "id"))
	for _, m := range matches {
		b, err := os.ReadFile(m)
		if err != nil || strings.TrimSpace(string(b)) != id {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(filepath.Base(filepath.Dir(m)), "card%d", &n); err == nil {
			return n, true
		}
	}
	return 0, false
}
