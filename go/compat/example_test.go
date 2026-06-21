package compat_test

import (
	"fmt"

	"github.com/signalbreak-labs/cambium/go/compat"
)

// Example shows the one change a goyang port must make. The goyang-shaped Entry
// tree is loaded the familiar way (NewModules → Parse → Process → GetModule), but
// ordered traversal goes through Entry.Children() — which returns schema
// declaration order (z, m, a) — rather than ranging over the Entry.Dir map, which
// is alphabetical (a, m, z). Entry.Dir stays available for name lookups.
func Example() {
	const src = `module order-demo {
  namespace "urn:order-demo";
  prefix od;
  container top {
    leaf z { type string; }
    leaf m { type string; }
    leaf a { type string; }
  }
}`
	ms := compat.NewModules()
	if err := ms.Parse(src, "order-demo"); err != nil {
		panic(err)
	}
	if errs := ms.Process(); len(errs) > 0 {
		panic(errs[0])
	}
	entry, errs := ms.GetModule("order-demo")
	if len(errs) > 0 {
		panic(errs[0])
	}

	top := entry.Dir["top"] // name lookup on Dir is fine
	for _, child := range top.Children() {
		fmt.Println(child.Name)
	}
	// Output:
	// z
	// m
	// a
}
