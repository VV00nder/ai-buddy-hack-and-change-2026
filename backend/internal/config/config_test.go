package config

import "testing"

func TestLoad(t *testing.T) {
	env := map[string]string{"BUDDY_ADDR": ":9", "LLM_TEMPERATURE": "0.5", "APP_TZ": "Nowhere/Bad", "COOKIE_SECURE": "true"}
	b := LoadBuddy(func(k string) string { return env[k] })
	if b.Addr != ":9" || b.LLMTemperature != 0.5 || b.Location.String() != "UTC" || !b.CookieSecure || b.MsgRateLimit != 20 {
		t.Fatalf("%+v", b)
	}
	c := LoadCorp(func(string) string { return "" })
	if c.DemoPassword != "buddy2026" || c.Location.String() != "Europe/Moscow" {
		t.Fatalf("%+v", c)
	}
	_ = LoadBuddy(nil)
	_ = LoadCorp(nil)
}
