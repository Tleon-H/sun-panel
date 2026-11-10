package cmn

import "testing"

func TestSysVersionInfoTrimsTheVersionFileNewline(t *testing.T) {
	info := GetSysVersionInfo()
	if info.Version_code != 10 || info.Version != "1.3.0+leon.10" {
		t.Fatalf("version = %d %q", info.Version_code, info.Version)
	}
}
