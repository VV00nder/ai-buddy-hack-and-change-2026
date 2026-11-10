package flags

import "testing"

func TestFromEnv(t *testing.T) {
	f := FromEnv(func(k string) string { return map[string]string{"FF_LLM": "false", "FF_ACCESS_ACTIONS": "junk"}[k] })
	if f.LLM || !f.AccessActions || !f.AbsenceActions {
		t.Fatalf("%+v", f)
	}
	_ = FromEnv(nil)
}
