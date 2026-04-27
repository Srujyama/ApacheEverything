package mqtt

import (
	"encoding/json"
	"testing"
)

func TestManifest(t *testing.T) {
	m := New().Manifest()
	if m.ID != "mqtt" || m.Mode != "stream" {
		t.Fatalf("manifest mismatch: %+v", m)
	}
	if len(m.ConfigSchema) == 0 {
		t.Fatal("missing config schema")
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		wantOK bool
	}{
		{"empty", ``, false},
		{"missing broker", `{"topics":["a"]}`, false},
		{"missing topics", `{"broker":"tcp://x:1883"}`, false},
		{"empty topics", `{"broker":"tcp://x:1883","topics":[]}`, false},
		{"qos out of range", `{"broker":"tcp://x:1883","topics":["a"],"qos":3}`, false},
		{"ok minimal", `{"broker":"tcp://x:1883","topics":["sensors/+"]}`, true},
		{"ok full", `{"broker":"ssl://x:8883","topics":["a","b"],"qos":2,"clientId":"c","cleanSession":false}`, true},
	}
	c := New()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := c.Validate(json.RawMessage(tc.body))
			if (err == nil) != tc.wantOK {
				t.Fatalf("Validate(%q) ok=%v, want %v (err=%v)", tc.body, err == nil, tc.wantOK, err)
			}
		})
	}
}

func TestApplyDefaults(t *testing.T) {
	c := Config{}
	c.applyDefaults()
	if c.CleanSession == nil || !*c.CleanSession {
		t.Fatalf("CleanSession default: %+v", c.CleanSession)
	}
	c2 := Config{QoS: 5}
	c2.applyDefaults()
	if c2.QoS != 1 {
		t.Fatalf("QoS clamp: %d", c2.QoS)
	}
}
