package datatree_test

import (
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/datatree"
)

// dtXML is the dt fixture in XML: top-level elements each carry xmlns; nested
// elements inherit. Order is schema declaration order with list keys first.
const dtXML = `<c xmlns="urn:dt"><z>hi</z><m>7</m><a>true</a></c>` +
	`<tags xmlns="urn:dt">t1</tags><tags xmlns="urn:dt">t2</tags>` +
	`<item xmlns="urn:dt"><id>1</id><name>x</name></item>`

const dtJSON = `{"dt:c":{"z":"hi","m":7,"a":true},"dt:tags":["t1","t2"],"dt:item":[{"id":"1","name":"x"}]}`

func TestXMLRoundTrip(t *testing.T) {
	mod := loadDT(t)
	tree, err := datatree.Parse(mod, datatree.FormatXML, []byte(dtXML))
	if err != nil {
		t.Fatalf("Parse XML: %v", err)
	}
	out, err := tree.Serialize(datatree.FormatXML)
	if err != nil {
		t.Fatalf("Serialize XML: %v", err)
	}
	if got := string(out); got != dtXML {
		t.Fatalf("XML round-trip mismatch:\n got: %s\nwant: %s", got, dtXML)
	}
}

func TestCrossFormatJSONToXML(t *testing.T) {
	mod := loadDT(t)
	tree, err := datatree.Parse(mod, datatree.FormatJSONIETF, []byte(dtJSON))
	if err != nil {
		t.Fatalf("Parse JSON: %v", err)
	}
	out, err := tree.Serialize(datatree.FormatXML)
	if err != nil {
		t.Fatalf("Serialize XML: %v", err)
	}
	if got := string(out); got != dtXML {
		t.Fatalf("JSON->XML mismatch:\n got: %s\nwant: %s", got, dtXML)
	}
}

func TestCrossFormatXMLToJSON(t *testing.T) {
	mod := loadDT(t)
	tree, err := datatree.Parse(mod, datatree.FormatXML, []byte(dtXML))
	if err != nil {
		t.Fatalf("Parse XML: %v", err)
	}
	out, err := tree.Serialize(datatree.FormatJSONIETF)
	if err != nil {
		t.Fatalf("Serialize JSON: %v", err)
	}
	if got := string(out); got != dtJSON {
		t.Fatalf("XML->JSON mismatch:\n got: %s\nwant: %s", got, dtJSON)
	}
}

// TestXMLValidation proves an XML-parsed tree validates like a JSON-parsed one
// (the internal representation is the same JSON token): an out-of-range int is
// caught.
func TestXMLValidation(t *testing.T) {
	mod := loadTC(t)
	tree, err := datatree.Parse(mod, datatree.FormatXML, []byte(`<n xmlns="urn:tc">200</n>`))
	if err != nil {
		t.Fatalf("Parse XML: %v", err)
	}
	err = tree.Validate()
	if err == nil || !strings.Contains(err.Error(), "range") {
		t.Fatalf("expected range violation for n=200, got %v", err)
	}
}

func TestXMLUnknownElementRejected(t *testing.T) {
	mod := loadDT(t)
	_, err := datatree.Parse(mod, datatree.FormatXML, []byte(`<bogus xmlns="urn:dt">x</bogus>`))
	if err == nil || !strings.Contains(err.Error(), "unknown XML element") {
		t.Fatalf("expected unknown-element error, got %v", err)
	}
}
