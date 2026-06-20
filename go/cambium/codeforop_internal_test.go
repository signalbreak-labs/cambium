package cambium

import "testing"

// TestCodeForOpReadOps pins the rule-code mapping for the data-read/navigation
// op strings. These error paths are hard to force through the public API on a
// valid handle, so the mapping is asserted directly: a miss here previously fell
// through to RuleCodeUnknown (E0000) instead of the spec-mandated codes.
func TestCodeForOpReadOps(t *testing.T) {
	cases := map[string]RuleCode{
		"children":          RuleCodeDataPath,    // E0006
		"siblings":          RuleCodeDataPath,    // E0006
		"schema":            RuleCodeDataPath,    // E0006
		"value":             RuleCodeDataPath,    // E0006
		"user ordered view": RuleCodeOrderedList, // E0005
		"diff":              RuleCodeDataPath,    // E0006
		"diff apply":        RuleCodeDataPath,    // E0006
		"merge":             RuleCodeValidate,    // E0003
		"duplicate":         RuleCodeSerialize,   // E0004
	}
	for op, want := range cases {
		if got := codeForOp(op); got != want {
			t.Errorf("codeForOp(%q) = %v, want %v", op, got, want)
		}
	}
}
