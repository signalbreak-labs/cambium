package datatree_test

import (
	"strings"
	"testing"
)

func TestAugmentedMustUnprefixedQNameUsesAugmentNamespace(t *testing.T) {
	base := `module ax-base {
	    namespace "urn:ax-base"; prefix b;
	    container c {
	        leaf flag { type string; }
	    }
	}`
	aug := `module ax-aug {
	    namespace "urn:ax-aug"; prefix a;
	    import ax-base { prefix b; }
	    augment "/b:c" {
	        leaf flag { type string; }
	        leaf probe {
	            must "../flag = 'yes'";
	            type string;
	        }
	    }
	}`
	mod := loadMultiModSrc(t, "ax-base", base, aug)

	if err := validateOne(t, mod, `{"ax-base:c":{"flag":"no","ax-aug:flag":"yes","ax-aug:probe":"x"}}`); err != nil {
		t.Fatalf("unprefixed XPath in augmented node should match augment namespace: %v", err)
	}
	err := validateOne(t, mod, `{"ax-base:c":{"flag":"yes","ax-aug:flag":"no","ax-aug:probe":"x"}}`)
	if err == nil || !strings.Contains(err.Error(), "must condition") {
		t.Fatalf("unprefixed XPath in augmented node should not match base same-local leaf, got %v", err)
	}
}

func TestAugmentWhenXPathUsesTargetContextAndPrefixes(t *testing.T) {
	base := `module ax-when-base {
	    namespace "urn:ax-when-base"; prefix b;
	    container system {
	        leaf mode { type string; }
	        container ospf {
	            leaf router-id { type string; }
	        }
	    }
	}`
	aug := `module ax-when-aug {
	    namespace "urn:ax-when-aug"; prefix a;
	    import ax-when-base { prefix b; }
	    augment "/b:system/b:ospf" {
	        when "../b:mode = 'enabled'";
	        leaf area { type string; }
	    }
	}`
	mod := loadMultiModSrc(t, "ax-when-base", base, aug)

	if err := validateOne(t, mod, `{"ax-when-base:system":{"mode":"enabled","ospf":{"router-id":"1.1.1.1","ax-when-aug:area":"0.0.0.0"}}}`); err != nil {
		t.Fatalf("augment when should resolve base-prefixed sibling from target context: %v", err)
	}
	err := validateOne(t, mod, `{"ax-when-base:system":{"mode":"disabled","ospf":{"router-id":"1.1.1.1","ax-when-aug:area":"0.0.0.0"}}}`)
	if err == nil || !strings.Contains(err.Error(), "when condition") {
		t.Fatalf("present augmented node should violate false augment when, got %v", err)
	}
}

func TestAugmentWhenUnprefixedQNameUsesTargetNamespace(t *testing.T) {
	base := `module ax-when-unprefixed-base {
	    namespace "urn:ax-when-unprefixed-base"; prefix b;
	    container system {
	        container ospf {
	            leaf mode { type string; }
	        }
	    }
	}`
	aug := `module ax-when-unprefixed-aug {
	    namespace "urn:ax-when-unprefixed-aug"; prefix a;
	    import ax-when-unprefixed-base { prefix b; }
	    augment "/b:system/b:ospf" {
	        when "mode = 'enabled'";
	        leaf mode { type string; }
	        leaf area { type string; }
	    }
	}`
	mod := loadMultiModSrc(t, "ax-when-unprefixed-base", base, aug)

	if err := validateOne(t, mod, `{"ax-when-unprefixed-base:system":{"ospf":{"mode":"enabled","ax-when-unprefixed-aug:mode":"disabled","ax-when-unprefixed-aug:area":"0.0.0.0"}}}`); err != nil {
		t.Fatalf("unprefixed augment when should use target namespace: %v", err)
	}
	err := validateOne(t, mod, `{"ax-when-unprefixed-base:system":{"ospf":{"mode":"disabled","ax-when-unprefixed-aug:mode":"enabled","ax-when-unprefixed-aug:area":"0.0.0.0"}}}`)
	if err == nil || !strings.Contains(err.Error(), "when condition") {
		t.Fatalf("unprefixed augment when should not use augment child namespace, got %v", err)
	}
}

func TestAugmentWhenThroughChoiceUsesTargetContext(t *testing.T) {
	base := `module ax-choice-base {
	    namespace "urn:ax-choice-base"; prefix b;
	    container system {
	        leaf mode { type string; }
	        container proto { }
	    }
	}`
	aug := `module ax-choice-aug {
	    namespace "urn:ax-choice-aug"; prefix a;
	    import ax-choice-base { prefix b; }
	    augment "/b:system/b:proto" {
	        when "../b:mode = 'enabled'";
	        choice detail {
	            case ospf {
	                leaf area { type string; }
	            }
	        }
	    }
	}`
	mod := loadMultiModSrc(t, "ax-choice-base", base, aug)

	if err := validateOne(t, mod, `{"ax-choice-base:system":{"mode":"enabled","proto":{"ax-choice-aug:area":"0.0.0.0"}}}`); err != nil {
		t.Fatalf("augment when through choice/case should use target context: %v", err)
	}
	err := validateOne(t, mod, `{"ax-choice-base:system":{"mode":"disabled","proto":{"ax-choice-aug:area":"0.0.0.0"}}}`)
	if err == nil || !strings.Contains(err.Error(), "when condition") {
		t.Fatalf("false augment when through choice/case should be reported, got %v", err)
	}
}

func TestAugmentXPathAbsolutePathFindsAugmentedNode(t *testing.T) {
	base := `module ax-abs-base {
	    namespace "urn:ax-abs-base"; prefix b;
	    container c {
	        leaf mode { type string; }
	    }
	}`
	aug := `module ax-abs-aug {
	    namespace "urn:ax-abs-aug"; prefix a;
	    import ax-abs-base { prefix b; }
	    augment "/b:c" {
	        leaf target { type string; }
	        leaf guard {
	            must "count(/b:c/a:target) = 1";
	            type string;
	        }
	    }
	}`
	mod := loadMultiModSrc(t, "ax-abs-base", base, aug)

	if err := validateOne(t, mod, `{"ax-abs-base:c":{"mode":"up","ax-abs-aug:target":"present","ax-abs-aug:guard":"x"}}`); err != nil {
		t.Fatalf("absolute XPath should traverse augmented child by module prefix: %v", err)
	}
	err := validateOne(t, mod, `{"ax-abs-base:c":{"mode":"up","ax-abs-aug:guard":"x"}}`)
	if err == nil || !strings.Contains(err.Error(), "must condition") {
		t.Fatalf("absolute XPath should report absent augmented target, got %v", err)
	}
}

func TestInstanceIdentifierToAugmentedNode(t *testing.T) {
	base := `module ax-iid-base {
	    namespace "urn:ax-iid-base"; prefix b;
	    container c {
	        leaf mode { type string; }
	    }
	}`
	aug := `module ax-iid-aug {
	    yang-version 1.1;
	    namespace "urn:ax-iid-aug"; prefix a;
	    import ax-iid-base { prefix b; }
	    augment "/b:c" {
	        leaf target { type string; }
	        leaf ptr {
	            type instance-identifier {
	                require-instance true;
	            }
	        }
	    }
	}`
	mod := loadMultiModSrc(t, "ax-iid-base", base, aug)

	if err := validateOne(t, mod, `{"ax-iid-base:c":{"mode":"up","ax-iid-aug:target":"present","ax-iid-aug:ptr":"/b:c/a:target"}}`); err != nil {
		t.Fatalf("instance-identifier should resolve augmented target by module prefix: %v", err)
	}
	err := validateOne(t, mod, `{"ax-iid-base:c":{"mode":"up","ax-iid-aug:ptr":"/b:c/a:target"}}`)
	if err == nil || !strings.Contains(err.Error(), "non-existent instance") {
		t.Fatalf("missing augmented instance-id target should be reported, got %v", err)
	}
}
