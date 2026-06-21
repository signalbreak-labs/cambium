// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package yangparse

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	upstream "github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream/yang"
)

// TestNativeParserMatchesUpstreamOnCorpus is a clean-room equivalence guard: for
// every YANG file in the shared conformance corpus that both parsers accept, the
// Cambium-native parser must produce a Statement tree byte-identical to goyang's
// (keyword, argument, has-argument, and ordered children, recursively). This pins
// the native parser to the behavior the conformance golden outputs were generated
// against. goyang is used here only as a black-box oracle (compared by output),
// never as an implementation reference.
func TestNativeParserMatchesUpstreamOnCorpus(t *testing.T) {
	root := filepath.Join("..", "..", "..", "conformance")
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".yang") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk corpus: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no corpus .yang files found")
	}

	compared := 0
	skipped := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		src := string(data)
		native, nerr := Parse(src, f)
		up, uerr := upstream.Parse(src, f)
		if nerr != nil || uerr != nil {
			skipped++
			t.Errorf("%s: parser accept/reject mismatch or shared rejection: native err=%v upstream err=%v", f, nerr, uerr)
			continue
		}
		if err := compareStatementLists(native, up); err != nil {
			t.Errorf("%s: native parser diverged from goyang: %v", f, err)
		}
		compared++
	}
	t.Logf("differential corpus comparison: %d/%d files compared (both parsers accepted)", compared, len(files))
	if skipped != 0 || compared != len(files) {
		t.Fatalf("differential test skipped %d/%d corpus files; add an explicit allowlist before intentional parser rejection divergence", skipped, len(files))
	}
}

func compareStatementLists(native []*Statement, up []*upstream.Statement) error {
	if len(native) != len(up) {
		return fmt.Errorf("top-level statement count native=%d upstream=%d", len(native), len(up))
	}
	for i := range native {
		if err := compareStatement(native[i], up[i], "stmts["+strconv.Itoa(i)+"]"); err != nil {
			return err
		}
	}
	return nil
}

func compareStatement(n *Statement, u *upstream.Statement, path string) error {
	here := path + "/" + n.Keyword
	if n.Keyword != u.Keyword {
		return fmt.Errorf("%s: keyword native=%q upstream=%q", path, n.Keyword, u.Keyword)
	}
	if n.HasArgument != u.HasArgument {
		return fmt.Errorf("%s: HasArgument native=%v upstream=%v", here, n.HasArgument, u.HasArgument)
	}
	if n.Argument != u.Argument {
		return fmt.Errorf("%s: argument native=%q upstream=%q", here, n.Argument, u.Argument)
	}
	nc, uc := n.SubStatements(), u.SubStatements()
	if len(nc) != len(uc) {
		return fmt.Errorf("%s: child count native=%d upstream=%d", here, len(nc), len(uc))
	}
	for i := range nc {
		if err := compareStatement(nc[i], uc[i], here); err != nil {
			return err
		}
	}
	return nil
}
