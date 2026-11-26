package server

import "testing"

func TestUpdatesDisabledByDefault(t *testing.T) {
	t.Setenv("ARTEX_UPDATES", "")
	if updatesEnabled() {
		t.Fatal("updates must be off by default")
	}
	for _, v := range []string{"0", "false", "no", "off"} {
		t.Setenv("ARTEX_UPDATES", v)
		if updatesEnabled() {
			t.Fatalf("ARTEX_UPDATES=%q should stay off", v)
		}
	}
	for _, v := range []string{"1", "true", "on", "yes"} {
		t.Setenv("ARTEX_UPDATES", v)
		if !updatesEnabled() {
			t.Fatalf("ARTEX_UPDATES=%q should enable updates", v)
		}
	}
}
