package cambium_test

import (
	"os"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
)

func TestRuleCodeConstantsMatchSpecRegistry(t *testing.T) {
	registry := readRuleCodeRegistry(t)
	goCodes := map[string]cambium.RuleCode{
		"Unknown":     cambium.RuleCodeUnknown,
		"Context":     cambium.RuleCodeContext,
		"Parse":       cambium.RuleCodeParse,
		"Validate":    cambium.RuleCodeValidate,
		"Serialize":   cambium.RuleCodeSerialize,
		"OrderedList": cambium.RuleCodeOrderedList,
		"DataPath":    cambium.RuleCodeDataPath,
		"Stale":       cambium.RuleCodeStale,
	}

	for name, code := range goCodes {
		want, ok := registry[name]
		if !ok {
			t.Fatalf("Go rule code %s missing from spec registry", name)
		}
		if got := string(code); got != want {
			t.Fatalf("RuleCode%s = %s, want spec registry code %s", name, got, want)
		}
	}
	for name, code := range registry {
		if _, ok := goCodes[name]; !ok {
			t.Fatalf("spec registry code %s (%s) has no exported Go RuleCode constant", name, code)
		}
	}
}

func readRuleCodeRegistry(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(repoRoot(t) + "/spec/rule-codes.md")
	if err != nil {
		t.Fatal(err)
	}
	registry := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Split(line, "|")
		if len(fields) < 4 {
			continue
		}
		code := strings.Trim(strings.TrimSpace(fields[1]), "`")
		if !strings.HasPrefix(code, "CAMBIUM_E") {
			continue
		}
		name := strings.TrimSpace(fields[2])
		registry[name] = code
	}
	if len(registry) == 0 {
		t.Fatal("no rule codes found in spec registry")
	}
	return registry
}
