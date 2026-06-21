// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package datatree_test

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	"github.com/signalbreak-labs/cambium/go/datatree"
)

func loadVendorFeatureDeviationFixture(t *testing.T) cambium.Module {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	moduleDir := filepath.Clean(filepath.Join(
		filepath.Dir(file),
		"..",
		"..",
		"conformance",
		"fixtures",
		"schema-vendor-nokia-feature-deviation",
		"module",
	))

	builder, err := cambium.NewContextBuilder(cambium.ContextFlags{})
	if err != nil {
		t.Fatalf("NewContextBuilder: %v", err)
	}
	if err := builder.SearchPath(moduleDir); err != nil {
		t.Fatalf("SearchPath: %v", err)
	}
	if err := builder.LoadModule("schema-vendor-nokia-feature-base", nil, nil); err != nil {
		t.Fatalf("Load base: %v", err)
	}
	if err := builder.LoadModule("schema-vendor-nokia-feature-augment", nil, []string{"enhanced"}); err != nil {
		t.Fatalf("Load augment: %v", err)
	}
	if err := builder.LoadModule("schema-vendor-nokia-feature-dev", nil, []string{"hardware-profile"}); err != nil {
		t.Fatalf("Load deviation: %v", err)
	}
	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { ctx.Close() })

	mod, err := ctx.Schema("schema-vendor-nokia-feature-base")
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	return mod
}

func TestVendorFeatureDeviationFixtureValidation(t *testing.T) {
	mod := loadVendorFeatureDeviationFixture(t)

	valid := `{
	  "schema-vendor-nokia-feature-base:top": {
	    "system": {
	      "interface": [
	        {
	          "schema-vendor-nokia-feature-augment:telemetry-profile": "profile-b",
	          "schema-vendor-nokia-feature-augment:subinterface": ["0"],
	          "if-index": "index-b",
	          "admin-state": "enable",
	          "name": "if-b"
	        },
	        {
	          "schema-vendor-nokia-feature-augment:subinterface": ["0"],
	          "schema-vendor-nokia-feature-augment:qos": {},
	          "schema-vendor-nokia-feature-augment:mirror-source": "if-b",
	          "schema-vendor-nokia-feature-augment:telemetry-profile": "profile-a",
	          "if-index": "index-a",
	          "admin-state": "enable",
	          "name": "if-a"
	        }
	      ],
	      "hardware": {
	        "serial": "abc123"
	      },
	      "lag-member": ["if-a"]
	    },
	    "primary-interface": "if-a",
	    "mode": "enabled"
	  }
	}`
	tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(valid))
	if err != nil {
		t.Fatalf("Parse valid fixture data: %v", err)
	}
	if err := tree.Validate(); err != nil {
		t.Fatalf("Validate valid fixture data: %v", err)
	}
	out, err := tree.Serialize(datatree.FormatJSONIETF)
	if err != nil {
		t.Fatalf("Serialize valid fixture data: %v", err)
	}
	wantJSON := `{"schema-vendor-nokia-feature-base:top":{"mode":"enabled","primary-interface":"if-a","system":{"interface":[{"name":"if-b","admin-state":"enable","if-index":"index-b","schema-vendor-nokia-feature-augment:telemetry-profile":"profile-b","schema-vendor-nokia-feature-augment:subinterface":["0"]},{"name":"if-a","admin-state":"enable","if-index":"index-a","schema-vendor-nokia-feature-augment:telemetry-profile":"profile-a","schema-vendor-nokia-feature-augment:mirror-source":"if-b","schema-vendor-nokia-feature-augment:subinterface":["0"],"schema-vendor-nokia-feature-augment:qos":{}}],"lag-member":["if-a"],"hardware":{"serial":"abc123"}}}}`
	if got := string(out); got != wantJSON {
		t.Fatalf("serialized vendor fixture order mismatch:\n got: %s\nwant: %s", got, wantJSON)
	}

	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "deviation min-elements",
			in: `{
			  "schema-vendor-nokia-feature-base:top": {
			    "mode": "enabled",
			    "system": {
			      "interface": [
			        {
			          "name": "if-a",
			          "schema-vendor-nokia-feature-augment:telemetry-profile": "profile-a",
			          "schema-vendor-nokia-feature-augment:subinterface": ["0"]
			        }
			      ],
			      "lag-member": ["if-a"]
			    }
			  }
			}`,
			want: "fewer than min-elements",
		},
		{
			name: "conditional mandatory augment",
			in: `{
			  "schema-vendor-nokia-feature-base:top": {
			    "mode": "enabled",
			    "system": {
			      "interface": [
			        {
			          "name": "if-a",
			          "schema-vendor-nokia-feature-augment:telemetry-profile": "profile-a",
			          "schema-vendor-nokia-feature-augment:subinterface": ["0"]
			        },
			        {
			          "name": "if-b",
			          "schema-vendor-nokia-feature-augment:subinterface": ["0"]
			        }
			      ],
			      "lag-member": ["if-a"]
			    }
			  }
			}`,
			want: "missing mandatory",
		},
		{
			name: "augmented relative leafref",
			in: `{
			  "schema-vendor-nokia-feature-base:top": {
			    "mode": "enabled",
			    "system": {
			      "interface": [
			        {
			          "name": "if-a",
			          "schema-vendor-nokia-feature-augment:telemetry-profile": "profile-a",
			          "schema-vendor-nokia-feature-augment:mirror-source": "missing",
			          "schema-vendor-nokia-feature-augment:subinterface": ["0"]
			        },
			        {
			          "name": "if-b",
			          "schema-vendor-nokia-feature-augment:telemetry-profile": "profile-b",
			          "schema-vendor-nokia-feature-augment:subinterface": ["0"]
			        }
			      ],
			      "lag-member": ["if-a"]
			    }
			  }
			}`,
			want: "no matching instance",
		},
		{
			name: "augmented must",
			in: `{
			  "schema-vendor-nokia-feature-base:top": {
			    "mode": "enabled",
			    "system": {
			      "interface": [
			        {
			          "name": "if-a",
			          "schema-vendor-nokia-feature-augment:telemetry-profile": "",
			          "schema-vendor-nokia-feature-augment:subinterface": ["0"],
			          "schema-vendor-nokia-feature-augment:qos": {}
			        },
			        {
			          "name": "if-b",
			          "schema-vendor-nokia-feature-augment:telemetry-profile": "profile-b",
			          "schema-vendor-nokia-feature-augment:subinterface": ["0"]
			        }
			      ],
			      "lag-member": ["if-a"]
			    }
			  }
			}`,
			want: "must",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(tc.in))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			err = tree.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate error = %v, want substring %q", err, tc.want)
			}
		})
	}
}
