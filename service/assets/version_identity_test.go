package assets

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestVersionFileKeepsUpstreamNumericCode(t *testing.T) {
	raw, err := os.ReadFile("version")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(strings.TrimSpace(string(raw)), "|")
	if len(parts) != 2 {
		t.Fatalf("version file must stay code|name, got %q", raw)
	}
	code, err := strconv.Atoi(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	if code != 10 {
		t.Fatalf("numeric version code must stay 10 so existing comparisons are unchanged, got %d", code)
	}
	if parts[1] != "1.3.0+leon.4" {
		t.Fatalf("display version = %s", parts[1])
	}
	if strings.Contains(parts[1], "1.8.1") || strings.Contains(parts[1], "2.0") {
		t.Fatalf("this tree must not identify itself as a later official release: %s", parts[1])
	}
}
