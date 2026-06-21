package datatree_test

import (
	"strings"
	"testing"
)

func TestMustPrefixedQNameUsesNamespace(t *testing.T) {
	base := `module qbase {
	    namespace "urn:qbase"; prefix b;
	    container c {
	        leaf flag { type string; }
	    }
	}`
	aug := `module qaug {
	    namespace "urn:qaug"; prefix a;
	    import qbase { prefix b; }
	    augment "/b:c" {
	        leaf flag { type string; }
	        leaf probe {
	            must "../b:flag = 'yes'";
	            type string;
	        }
	    }
	}`
	mod := loadMultiModSrc(t, "qbase", base, aug)

	if err := validateOne(t, mod, `{"qbase:c":{"flag":"yes","qaug:flag":"no","qaug:probe":"x"}}`); err != nil {
		t.Fatalf("prefixed must path should match qbase:flag, not qaug:flag: %v", err)
	}
	err := validateOne(t, mod, `{"qbase:c":{"flag":"no","qaug:flag":"yes","qaug:probe":"x"}}`)
	if err == nil || !strings.Contains(err.Error(), "must condition") {
		t.Fatalf("prefixed must path should not match same-local augmented leaf, got %v", err)
	}
}

func TestLeafRefPrefixedQNameUsesModule(t *testing.T) {
	base := `module lrbase {
	    namespace "urn:lrbase"; prefix b;
	    container c {
	        list item {
	            key id;
	            leaf id { type string; }
	        }
	    }
	}`
	aug := `module laug {
	    namespace "urn:laug"; prefix a;
	    import lrbase { prefix b; }
	    augment "/b:c" {
	        list item {
	            key id;
	            leaf id { type string; }
	            leaf ref {
	                type leafref { path "../../a:item/a:id"; }
	            }
	        }
	    }
	}`
	mod := loadMultiModSrc(t, "lrbase", base, aug)

	err := validateOne(t, mod, `{"lrbase:c":{"item":[{"id":"wrong"}],"laug:item":[{"id":"ok","ref":"ok"}]}}`)
	if err != nil {
		t.Fatalf("prefixed leafref path should match augmented list, not base list: %v", err)
	}
}

func TestXPathNumericPredicateUsesExactPosition(t *testing.T) {
	mod := loadModSrc(t, `module xpred {
	    namespace "urn:xpred"; prefix xp;
	    list item {
	        key id;
	        leaf id { type string; }
	    }
	    leaf guard {
	        must "count(/xp:item[1.5]) = 0";
	        type string;
	    }
	}`, "xpred")

	if err := validateOne(t, mod, `{"xpred:item":[{"id":"a"},{"id":"b"}],"xpred:guard":"x"}`); err != nil {
		t.Fatalf("numeric predicate 1.5 should match no position exactly: %v", err)
	}
}

func TestXPathSubstringRoundsEndPoint(t *testing.T) {
	mod := loadModSrc(t, `module xsub {
	    namespace "urn:xsub"; prefix xs;
	    leaf guard {
	        must "substring('12345', 1.5, 2.6) = '23'";
	        type string;
	    }
	}`, "xsub")

	if err := validateOne(t, mod, `{"xsub:guard":"x"}`); err != nil {
		t.Fatalf("substring should round start+length as the endpoint: %v", err)
	}
}

func TestXPathLocalNameRequiresNodeset(t *testing.T) {
	mod := loadModSrc(t, `module xlocal {
	    namespace "urn:xlocal"; prefix xl;
	    leaf guard {
	        must "local-name('not-a-node-set') = 'guard'";
	        type string;
	    }
	}`, "xlocal")

	if err := validateOne(t, mod, `{"xlocal:guard":"x"}`); err != nil {
		if strings.Contains(err.Error(), "must") {
			t.Fatalf("local-name() with a non-node-set argument should skip, not false-reject: %v", err)
		}
	}
}
