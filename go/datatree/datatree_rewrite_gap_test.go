package datatree_test

import (
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/datatree"
)

func TestConditionalMandatoryAugmentSkippedWhenWhenFalse(t *testing.T) {
	base := `module rw-mand-base {
	    yang-version 1.1;
	    namespace "urn:rw-mand-base"; prefix base;
	    container top {
	        leaf mode { type string; }
	    }
	}`
	aug := `module rw-mand-aug {
	    yang-version 1.1;
	    namespace "urn:rw-mand-aug"; prefix aug;
	    import rw-mand-base { prefix base; }
	    augment "/base:top" {
	        when "base:mode = 'enabled'";
	        leaf required-name {
	            mandatory true;
	            type string;
	        }
	    }
	}`
	mod := loadMultiModSrc(t, "rw-mand-base", base, aug)

	if err := validateOne(t, mod, `{"rw-mand-base:top":{"mode":"disabled"}}`); err != nil {
		t.Fatalf("missing conditional mandatory augment should pass when augment when is false: %v", err)
	}
	err := validateOne(t, mod, `{"rw-mand-base:top":{"mode":"enabled"}}`)
	if err == nil || !strings.Contains(err.Error(), "missing mandatory") {
		t.Fatalf("missing conditional mandatory augment should fail when augment when is true, got %v", err)
	}
}

func TestConditionalMinElementsUsesSkippedWhenWhenFalse(t *testing.T) {
	mod := loadModSrc(t, `module rw-min-uses {
	    yang-version 1.1;
	    namespace "urn:rw-min-uses"; prefix rw;

	    grouping advanced {
	        list endpoint {
	            min-elements 1;
	            key name;
	            leaf name { type string; }
	        }
	    }

	    container top {
	        leaf mode { type string; }
	        uses advanced {
	            when "mode = 'enabled'";
	        }
	    }
	}`, "rw-min-uses")

	if err := validateOne(t, mod, `{"rw-min-uses:top":{"mode":"disabled"}}`); err != nil {
		t.Fatalf("missing min-elements list should pass when uses when is false: %v", err)
	}
	err := validateOne(t, mod, `{"rw-min-uses:top":{"mode":"enabled"}}`)
	if err == nil || !strings.Contains(err.Error(), "fewer than min-elements") {
		t.Fatalf("missing min-elements list should fail when uses when is true, got %v", err)
	}
}

func TestWhenEvaluationDoesNotSeeNodesAddedByAugment(t *testing.T) {
	base := `module rw-altered-base {
	    yang-version 1.1;
	    namespace "urn:rw-altered-base"; prefix base;
	    container top { }
	}`
	aug := `module rw-altered-aug {
	    yang-version 1.1;
	    namespace "urn:rw-altered-aug"; prefix aug;
	    import rw-altered-base { prefix base; }
	    augment "/base:top" {
	        when "not(aug:flag)";
	        leaf flag { type string; }
	    }
	}`
	mod := loadMultiModSrc(t, "rw-altered-base", base, aug)

	if err := validateOne(t, mod, `{"rw-altered-base:top":{"rw-altered-aug:flag":"present"}}`); err != nil {
		t.Fatalf("augment when should not see nodes added by that augment: %v", err)
	}
}

func TestDirectWhenUsesDummyContextNode(t *testing.T) {
	mod := loadModSrc(t, `module rw-dummy-when {
	    yang-version 1.1;
	    namespace "urn:rw-dummy-when"; prefix rw;
	    leaf flag {
	        when "string(.) = ''";
	        type string;
	    }
	}`, "rw-dummy-when")

	if err := validateOne(t, mod, `{"rw-dummy-when:flag":"present"}`); err != nil {
		t.Fatalf("direct when should evaluate against a dummy node with no value: %v", err)
	}
}

func TestDeviationReplacedLeafrefUsesDeviationPrefixContext(t *testing.T) {
	base := `module rw-leafref-base {
	    yang-version 1.1;
	    namespace "urn:rw-leafref-base"; prefix base;
	    container top {
	        leaf target { type string; }
	        leaf ptr { type string; }
	    }
	}`
	dev := `module rw-leafref-dev {
	    yang-version 1.1;
	    namespace "urn:rw-leafref-dev"; prefix dev;
	    import rw-leafref-base { prefix target; }
	    deviation "/target:top/target:ptr" {
	        deviate replace {
	            type leafref {
	                path "/target:top/target:target";
	                require-instance true;
	            }
	        }
	    }
	}`
	mod := loadMultiModSrc(t, "rw-leafref-base", base, dev)

	if err := validateOne(t, mod, `{"rw-leafref-base:top":{"target":"a","ptr":"a"}}`); err != nil {
		t.Fatalf("deviation-replaced leafref should resolve path with deviation module prefixes: %v", err)
	}
	err := validateOne(t, mod, `{"rw-leafref-base:top":{"target":"a","ptr":"missing"}}`)
	if err == nil || !strings.Contains(err.Error(), "leafref value") {
		t.Fatalf("deviation-replaced leafref should report missing target instance, got %v", err)
	}
}

func TestRefineAddedMustUsesRefiningModulePrefixContext(t *testing.T) {
	base := `module rw-refine-base {
	    yang-version 1.1;
	    namespace "urn:rw-refine-base"; prefix base;
	    grouping common {
	        leaf value { type string; }
	    }
	}`
	user := `module rw-refine-user {
	    yang-version 1.1;
	    namespace "urn:rw-refine-user"; prefix user;
	    import rw-refine-base { prefix grp; }

	    container top {
	        leaf guard { type string; }
	        uses grp:common {
	            refine value {
	                must "../user:guard = 'ok'";
	            }
	        }
	    }
	}`
	mod := loadMultiModSrc(t, "rw-refine-user", base, user)

	if err := validateOne(t, mod, `{"rw-refine-user:top":{"guard":"ok","value":"x"}}`); err != nil {
		t.Fatalf("refine-added must should resolve prefixes from refining module: %v", err)
	}
	err := validateOne(t, mod, `{"rw-refine-user:top":{"guard":"bad","value":"x"}}`)
	if err == nil || !strings.Contains(err.Error(), "must condition") {
		t.Fatalf("false refine-added must should be reported, got %v", err)
	}
}

func TestDeviationNotSupportedRemovesAugmentedChildAndPreservesOrder(t *testing.T) {
	base := `module rw-order-base {
	    yang-version 1.1;
	    namespace "urn:rw-order-base"; prefix base;
	    container top {
	        leaf a { type string; }
	        leaf b { type string; }
	    }
	}`
	aug := `module rw-order-aug {
	    yang-version 1.1;
	    namespace "urn:rw-order-aug"; prefix aug;
	    import rw-order-base { prefix base; }
	    augment "/base:top" {
	        leaf c { type string; }
	        leaf d { type string; }
	    }
	}`
	dev := `module rw-order-dev {
	    yang-version 1.1;
	    namespace "urn:rw-order-dev"; prefix dev;
	    import rw-order-aug { prefix aug; }
	    import rw-order-base { prefix base; }
	    deviation "/base:top/aug:c" {
	        deviate not-supported;
	    }
	}`
	mod := loadMultiModSrc(t, "rw-order-base", base, aug, dev)

	tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(`{"rw-order-base:top":{"rw-order-aug:d":"D","b":"B","a":"A"}}`))
	if err != nil {
		t.Fatalf("Parse without removed augment child: %v", err)
	}
	out, err := tree.Serialize(datatree.FormatJSONIETF)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	want := `{"rw-order-base:top":{"a":"A","b":"B","rw-order-aug:d":"D"}}`
	if string(out) != want {
		t.Fatalf("augment/deviation order mismatch:\n got: %s\nwant: %s", out, want)
	}

	_, err = datatree.Parse(mod, datatree.FormatJSONIETF, []byte(`{"rw-order-base:top":{"rw-order-aug:c":"removed"}}`))
	if err == nil || !strings.Contains(err.Error(), "unknown member") {
		t.Fatalf("removed augmented child should be rejected as unknown, got %v", err)
	}
}

func TestDeviationNotSupportedRemovesBaseNodeWithAugmentedChildren(t *testing.T) {
	base := `module rw-remove-base {
	    yang-version 1.1;
	    namespace "urn:rw-remove-base"; prefix base;
	    container top {
	        leaf a { type string; }
	    }
	}`
	aug := `module rw-remove-aug {
	    yang-version 1.1;
	    namespace "urn:rw-remove-aug"; prefix aug;
	    import rw-remove-base { prefix base; }
	    augment "/base:top" {
	        leaf extra { type string; }
	    }
	}`
	dev := `module rw-remove-dev {
	    yang-version 1.1;
	    namespace "urn:rw-remove-dev"; prefix dev;
	    import rw-remove-base { prefix base; }
	    deviation "/base:top" {
	        deviate not-supported;
	    }
	}`
	mod := loadMultiModSrc(t, "rw-remove-base", base, aug, dev)

	_, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(`{"rw-remove-base:top":{"a":"A","rw-remove-aug:extra":"E"}}`))
	if err == nil || !strings.Contains(err.Error(), "unknown member") {
		t.Fatalf("removed base node with augmented children should be rejected as unknown, got %v", err)
	}
}

func TestApplyDefaultsSkipsLeafWhenWhenFalse(t *testing.T) {
	mod := loadModSrc(t, `module rw-default-when {
	    yang-version 1.1;
	    namespace "urn:rw-default-when"; prefix rw;
	    container top {
	        leaf mode { type string; }
	        leaf value {
	            when "../mode = 'enabled'";
	            type string;
	            default "auto";
	        }
	    }
	}`, "rw-default-when")

	tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(`{"rw-default-when:top":{"mode":"disabled"}}`))
	if err != nil {
		t.Fatalf("Parse disabled: %v", err)
	}
	tree.ApplyDefaults()
	out, err := tree.Serialize(datatree.FormatJSONIETF)
	if err != nil {
		t.Fatalf("Serialize disabled: %v", err)
	}
	if got, want := string(out), `{"rw-default-when:top":{"mode":"disabled"}}`; got != want {
		t.Fatalf("false when default should not be applied:\n got: %s\nwant: %s", got, want)
	}

	tree, err = datatree.Parse(mod, datatree.FormatJSONIETF, []byte(`{"rw-default-when:top":{"mode":"enabled"}}`))
	if err != nil {
		t.Fatalf("Parse enabled: %v", err)
	}
	tree.ApplyDefaults()
	out, err = tree.Serialize(datatree.FormatJSONIETF)
	if err != nil {
		t.Fatalf("Serialize enabled: %v", err)
	}
	if got, want := string(out), `{"rw-default-when:top":{"mode":"enabled","value":"auto"}}`; got != want {
		t.Fatalf("true when default should be applied:\n got: %s\nwant: %s", got, want)
	}
}

func TestRefineIdentityRefDefaultUsesRefinePrefixContext(t *testing.T) {
	idents := `module rw-idref-refine-id {
	    yang-version 1.1;
	    namespace "urn:rw-idref-refine-id"; prefix id;
	    identity base;
	    identity derived { base base; }
	}`
	base := `module rw-idref-refine-base {
	    yang-version 1.1;
	    namespace "urn:rw-idref-refine-base"; prefix base;
	    import rw-idref-refine-id { prefix id; }
	    grouping common {
	        leaf mode {
	            type identityref {
	                base id:base;
	            }
	        }
	    }
	}`
	user := `module rw-idref-refine-user {
	    yang-version 1.1;
	    namespace "urn:rw-idref-refine-user"; prefix user;
	    import rw-idref-refine-base { prefix grp; }
	    import rw-idref-refine-id { prefix foreign; }
	    container top {
	        uses grp:common {
	            refine mode {
	                default "foreign:derived";
	            }
	        }
	    }
	}`
	mod := loadMultiModSrc(t, "rw-idref-refine-user", idents, base, user)

	tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(`{"rw-idref-refine-user:top":{}}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	tree.ApplyDefaults()
	out, err := tree.Serialize(datatree.FormatJSONIETF)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	if got, want := string(out), `{"rw-idref-refine-user:top":{"mode":"rw-idref-refine-id:derived"}}`; got != want {
		t.Fatalf("refine identityref default used wrong module context:\n got: %s\nwant: %s", got, want)
	}
}

func TestDeviationIdentityRefDefaultUsesDeviationPrefixContext(t *testing.T) {
	idents := `module rw-idref-dev-id {
	    yang-version 1.1;
	    namespace "urn:rw-idref-dev-id"; prefix id;
	    identity base;
	    identity derived { base base; }
	}`
	base := `module rw-idref-dev-base {
	    yang-version 1.1;
	    namespace "urn:rw-idref-dev-base"; prefix base;
	    import rw-idref-dev-id { prefix id; }
	    container top {
	        leaf mode {
	            type identityref {
	                base id:base;
	            }
	            default "id:base";
	        }
	    }
	}`
	dev := `module rw-idref-dev-source {
	    yang-version 1.1;
	    namespace "urn:rw-idref-dev-source"; prefix dev;
	    import rw-idref-dev-base { prefix target; }
	    import rw-idref-dev-id { prefix foreign; }
	    deviation "/target:top/target:mode" {
	        deviate replace {
	            default "foreign:derived";
	        }
	    }
	}`
	mod := loadMultiModSrc(t, "rw-idref-dev-base", idents, base, dev)

	tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(`{"rw-idref-dev-base:top":{}}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	tree.ApplyDefaults()
	out, err := tree.Serialize(datatree.FormatJSONIETF)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	if got, want := string(out), `{"rw-idref-dev-base:top":{"mode":"rw-idref-dev-id:derived"}}`; got != want {
		t.Fatalf("deviation identityref default used wrong module context:\n got: %s\nwant: %s", got, want)
	}
}
