package registry

import "testing"

func TestLoadBundled(t *testing.T) {
	d, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if d.Version == "" {
		t.Fatal("registry version empty")
	}
	if len(d.Connectors) == 0 {
		t.Fatal("registry has no connectors")
	}
	want := []string{
		"usgs-earthquakes",
		"noaa-weather-alerts",
		"usgs-water",
		"nasa-firms",
		"openaq",
		"webhook",
	}
	got := map[string]bool{}
	for _, c := range d.Connectors {
		got[c.ID] = true
	}
	for _, id := range want {
		if !got[id] {
			t.Errorf("registry missing expected connector %q", id)
		}
	}

	// Spot-check that secrets are populated for the auth-required ones.
	for _, c := range d.Connectors {
		if c.ID == "nasa-firms" && len(c.Secrets) == 0 {
			t.Errorf("nasa-firms registry entry should declare a secret")
		}
		if c.ID == "openaq" && len(c.Secrets) == 0 {
			t.Errorf("openaq registry entry should declare a secret")
		}
	}
}
