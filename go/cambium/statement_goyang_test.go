// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package cambium_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/signalbreak-labs/cambium/go/cambium"
	upstream "github.com/signalbreak-labs/cambium/go/internal/yangparse/upstream/yang"
)

func TestParseStatementsPublicAPIMatchesGoyangOnCorpus(t *testing.T) {
	root := filepath.Join(repoRoot(t), "conformance")
	files := yangCorpusFiles(t, root)
	if len(files) == 0 {
		t.Fatal("no corpus .yang files found")
	}

	for _, path := range files {
		t.Run(filepath.ToSlash(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read corpus file: %v", err)
			}
			source := string(data)
			native, nerr := cambium.ParseStatements(source, path)
			goyang, uerr := upstream.Parse(source, path)
			if nerr != nil || uerr != nil {
				t.Fatalf("parser accept/reject mismatch or shared rejection: native err=%v goyang err=%v", nerr, uerr)
			}
			if err := comparePublicStatementsToGoyang(native, goyang); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestParseStatementsPublicAPIMatchesGoyangEdgeCases(t *testing.T) {
	const bom = "\ufeff"
	tests := []struct {
		name   string
		source string
	}{
		{
			name:   "escaped double quoted argument",
			source: "module m { namespace \"u\"; prefix p; leaf l { type string; description \"a\\nb\\tc\\\"d\\\\e\"; } }\n",
		},
		{
			name:   "mixed quoted concatenation",
			source: "module m { namespace \"u\"; prefix p; leaf l { type string; description \"x\" + 'y' + \"z\"; } }\n",
		},
		{
			name:   "tabs in multiline quote indentation",
			source: "module m {\n\tnamespace \"u\";\n\tprefix p;\n\tdescription \"a\n\t            b\";\n}\n",
		},
		{
			name:   "bare CR in quoted argument",
			source: "module m {\n namespace \"u\"; prefix p;\n description \"a  \r b\";\n}\n",
		},
		{
			name:   "CRLF line endings",
			source: "module m {\r\n namespace \"u\";\r\n prefix p;\r\n description \"l1\r\n l2\";\r\n}\r\n",
		},
		{
			name:   "empty and comment only input",
			source: "// only a comment\n/* and a block */\n",
		},
		{
			name:   "leading BOM",
			source: bom + "module m { namespace \"u\"; prefix p; }\n",
		},
		{
			name:   "unquoted concatenation rejected",
			source: "module m { namespace \"u\"; prefix p; leaf l { type string; description foo + bar; } }\n",
		},
		{
			name:   "quoted plus unquoted rejected",
			source: "module m { namespace \"u\"; prefix p; leaf l { type string; default \"a\" + b; } }\n",
		},
		{
			name:   "hash comment rejected",
			source: "module m { namespace \"u\"; prefix p; # not a YANG comment\n}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			native, nerr := cambium.ParseStatements(tt.source, tt.name+".yang")
			goyang, uerr := upstream.Parse(tt.source, tt.name+".yang")
			if (nerr == nil) != (uerr == nil) {
				t.Fatalf("accept/reject mismatch: native err=%v goyang err=%v", nerr, uerr)
			}
			if nerr != nil {
				return
			}
			if err := comparePublicStatementsToGoyang(native, goyang); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestParseStatementsPublicAPILocationMatchesGoyang(t *testing.T) {
	const source = `module public-location {
  namespace "urn:public-location";
  prefix pl;

  container top {
    leaf value { type string; }
  }
}`

	native, err := cambium.ParseStatements(source, "public-location.yang")
	if err != nil {
		t.Fatalf("ParseStatements: %v", err)
	}
	goyang, err := upstream.Parse(source, "public-location.yang")
	if err != nil {
		t.Fatalf("goyang Parse: %v", err)
	}

	nativeLeaf := findPublicStatement(native, "leaf", "value")
	goyangLeaf := findGoyangStatement(goyang, "leaf", "value")
	if !nativeLeaf.IsValid() || goyangLeaf == nil {
		t.Fatalf("leaf statements = (%v,%#v), want both present", nativeLeaf.IsValid(), goyangLeaf)
	}
	if got, want := nativeLeaf.Location(), goyangLeaf.Location(); got != want {
		t.Fatalf("leaf Location = %q, want goyang %q", got, want)
	}
}

func TestParseStatementsPublicAPIRejectsInvalidUTF8(t *testing.T) {
	source := `module bad-utf8 {
  namespace "urn:bad-utf8";
  prefix bu;
  description "` + string([]byte{0xff}) + `";
}`

	if _, err := cambium.ParseStatements(source, "bad-utf8.yang"); err == nil {
		t.Fatal("ParseStatements accepted invalid UTF-8")
	}
}

func yangCorpusFiles(t *testing.T, root string) []string {
	t.Helper()
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
	return files
}

func comparePublicStatementsToGoyang(native []cambium.Statement, goyang []*upstream.Statement) error {
	if len(native) != len(goyang) {
		return fmt.Errorf("top-level statement count native=%d goyang=%d", len(native), len(goyang))
	}
	for i := range native {
		if err := comparePublicStatementToGoyang(native[i], goyang[i], "stmts["+strconv.Itoa(i)+"]"); err != nil {
			return err
		}
	}
	return nil
}

func comparePublicStatementToGoyang(native cambium.Statement, goyang *upstream.Statement, path string) error {
	if !native.IsValid() || goyang == nil {
		return fmt.Errorf("%s: statement validity native=%v goyang=%v", path, native.IsValid(), goyang != nil)
	}
	here := path + "/" + native.Keyword()
	if native.Keyword() != goyang.Keyword {
		return fmt.Errorf("%s: keyword native=%q goyang=%q", path, native.Keyword(), goyang.Keyword)
	}
	nativeArg, nativeHasArg := native.Argument()
	if nativeHasArg != goyang.HasArgument {
		return fmt.Errorf("%s: has-argument native=%v goyang=%v", here, nativeHasArg, goyang.HasArgument)
	}
	if nativeArg != goyang.Argument {
		return fmt.Errorf("%s: argument native=%q goyang=%q", here, nativeArg, goyang.Argument)
	}

	nativeChildren := native.SubStatements()
	goyangChildren := goyang.SubStatements()
	if len(nativeChildren) != len(goyangChildren) {
		return fmt.Errorf("%s: child count native=%d goyang=%d", here, len(nativeChildren), len(goyangChildren))
	}
	for i := range nativeChildren {
		if err := comparePublicStatementToGoyang(nativeChildren[i], goyangChildren[i], here+"["+strconv.Itoa(i)+"]"); err != nil {
			return err
		}
	}
	return nil
}

func findPublicStatement(stmts []cambium.Statement, keyword, arg string) cambium.Statement {
	for _, stmt := range stmts {
		stmtArg, hasArg := stmt.Argument()
		if stmt.Keyword() == keyword && ((!hasArg && arg == "") || stmtArg == arg) {
			return stmt
		}
		if found := findPublicStatement(stmt.SubStatements(), keyword, arg); found.IsValid() {
			return found
		}
	}
	return cambium.Statement{}
}

func findGoyangStatement(stmts []*upstream.Statement, keyword, arg string) *upstream.Statement {
	for _, stmt := range stmts {
		if stmt.Keyword == keyword && ((!stmt.HasArgument && arg == "") || stmt.Argument == arg) {
			return stmt
		}
		if found := findGoyangStatement(stmt.SubStatements(), keyword, arg); found != nil {
			return found
		}
	}
	return nil
}
