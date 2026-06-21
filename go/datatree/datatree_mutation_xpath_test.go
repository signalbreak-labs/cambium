package datatree_test

import (
	"strings"
	"testing"
)

func TestUsesWhenXPathUsesNearestDataAncestor(t *testing.T) {
	mod := loadModSrc(t, `module ux-uses-when {
	    namespace "urn:ux-uses-when"; prefix ux;

	    grouping gated {
	        leaf enabled-leaf { type string; }
	    }

	    container top {
	        leaf mode { type string; }
	        uses gated {
	            when "mode = 'enabled'";
	        }
	    }
	}`, "ux-uses-when")

	if err := validateOne(t, mod, `{"ux-uses-when:top":{"mode":"enabled","enabled-leaf":"present"}}`); err != nil {
		t.Fatalf("uses-level when should evaluate from nearest data ancestor: %v", err)
	}
	err := validateOne(t, mod, `{"ux-uses-when:top":{"mode":"disabled","enabled-leaf":"present"}}`)
	if err == nil || !strings.Contains(err.Error(), "when condition") {
		t.Fatalf("false uses-level when should be reported, got %v", err)
	}
}

func TestChoiceWhenXPathIsEnforcedOnFlattenedDescendant(t *testing.T) {
	mod := loadModSrc(t, `module ux-choice-when {
	    namespace "urn:ux-choice-when"; prefix ux;

	    container top {
	        leaf mode { type string; }
	        choice detail {
	            when "mode = 'enabled'";
	            leaf enabled-leaf { type string; }
	        }
	    }
	}`, "ux-choice-when")

	if err := validateOne(t, mod, `{"ux-choice-when:top":{"mode":"enabled","enabled-leaf":"present"}}`); err != nil {
		t.Fatalf("choice-level when should evaluate from nearest data ancestor: %v", err)
	}
	err := validateOne(t, mod, `{"ux-choice-when:top":{"mode":"disabled","enabled-leaf":"present"}}`)
	if err == nil || !strings.Contains(err.Error(), "when condition") {
		t.Fatalf("false choice-level when should be reported, got %v", err)
	}
}

func TestDeviationAddedMustUsesDeviationPrefixContext(t *testing.T) {
	base := `module ux-dev-base {
	    namespace "urn:ux-dev-base"; prefix base;

	    container top {
	        leaf guard { type string; }
	        leaf value { type string; }
	    }
	}`
	dev := `module ux-dev-source {
	    namespace "urn:ux-dev-source"; prefix dev;
	    import ux-dev-base { prefix target; }

	    deviation "/target:top" {
	        deviate add {
	            must "target:guard = 'ok'";
	        }
	    }
	}`
	mod := loadMultiModSrc(t, "ux-dev-base", base, dev)

	if err := validateOne(t, mod, `{"ux-dev-base:top":{"guard":"ok","value":"x"}}`); err != nil {
		t.Fatalf("deviation-added must should resolve prefixes from deviation module: %v", err)
	}
	err := validateOne(t, mod, `{"ux-dev-base:top":{"guard":"bad","value":"x"}}`)
	if err == nil || !strings.Contains(err.Error(), "must condition") {
		t.Fatalf("false deviation-added must should be reported, got %v", err)
	}
}
