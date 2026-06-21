package datatree

import (
	"fmt"
	"math"
	"strings"
)

// Core XPath 1.0 function library. Functions outside this set return an error so
// the caller skips the check (YANG-specific functions like derived-from,
// re-match, bit-is-set, and deref are intentionally not implemented yet and
// therefore skip rather than mis-evaluate).
func (ev *evaluator) evalCall(x *xCall, c ectx) (xval, error) {
	args := make([]xval, len(x.args))
	for i, a := range x.args {
		v, err := ev.eval(a, c)
		if err != nil {
			return xval{}, err
		}
		args[i] = v
	}
	arity := func(want int) error {
		if len(args) != want {
			return fmt.Errorf("%s() expects %d argument(s), got %d", x.name, want, len(args))
		}
		return nil
	}

	switch x.name {
	case "true":
		return boolVal(true), arity(0)
	case "false":
		return boolVal(false), arity(0)
	case "not":
		if err := arity(1); err != nil {
			return xval{}, err
		}
		return boolVal(!args[0].toBool()), nil
	case "boolean":
		if err := arity(1); err != nil {
			return xval{}, err
		}
		return boolVal(args[0].toBool()), nil
	case "string":
		if len(args) == 0 {
			return strVal(stringValue(c.node)), nil
		}
		return strVal(args[0].toStr()), arity(1)
	case "number":
		if len(args) == 0 {
			return numVal(parseXPathNumber(stringValue(c.node))), nil
		}
		return numVal(args[0].toNum()), arity(1)
	case "count":
		if err := arity(1); err != nil {
			return xval{}, err
		}
		if args[0].kind != kNodeset {
			return xval{}, fmt.Errorf("count() requires a node-set")
		}
		return numVal(float64(len(args[0].ns))), nil
	case "sum":
		if err := arity(1); err != nil {
			return xval{}, err
		}
		if args[0].kind != kNodeset {
			return xval{}, fmt.Errorf("sum() requires a node-set")
		}
		total := 0.0
		for _, n := range args[0].ns {
			total += parseXPathNumber(stringValue(n))
		}
		return numVal(total), nil
	case "position":
		return numVal(float64(c.pos)), arity(0)
	case "last":
		return numVal(float64(c.size)), arity(0)
	case "contains":
		if err := arity(2); err != nil {
			return xval{}, err
		}
		return boolVal(strings.Contains(args[0].toStr(), args[1].toStr())), nil
	case "starts-with":
		if err := arity(2); err != nil {
			return xval{}, err
		}
		return boolVal(strings.HasPrefix(args[0].toStr(), args[1].toStr())), nil
	case "string-length":
		s := stringValue(c.node)
		if len(args) == 1 {
			s = args[0].toStr()
		} else if len(args) > 1 {
			return xval{}, fmt.Errorf("string-length() takes at most 1 argument")
		}
		return numVal(float64(len([]rune(s)))), nil
	case "concat":
		if len(args) < 2 {
			return xval{}, fmt.Errorf("concat() needs at least 2 arguments")
		}
		var b strings.Builder
		for _, a := range args {
			b.WriteString(a.toStr())
		}
		return strVal(b.String()), nil
	case "substring":
		if len(args) < 2 || len(args) > 3 {
			return xval{}, fmt.Errorf("substring() takes 2 or 3 arguments")
		}
		return strVal(xpathSubstring(args)), nil
	case "floor":
		if err := arity(1); err != nil {
			return xval{}, err
		}
		return numVal(math.Floor(args[0].toNum())), nil
	case "ceiling":
		if err := arity(1); err != nil {
			return xval{}, err
		}
		return numVal(math.Ceil(args[0].toNum())), nil
	case "round":
		if err := arity(1); err != nil {
			return xval{}, err
		}
		return numVal(xpathRound(args[0].toNum())), nil
	case "current":
		if err := arity(0); err != nil {
			return xval{}, err
		}
		if ev.current == nil {
			return xval{kind: kNodeset}, nil
		}
		return xval{kind: kNodeset, ns: []*xnode{ev.current}}, nil
	case "local-name":
		var n *xnode
		switch {
		case len(args) == 0:
			n = c.node
		case args[0].kind == kNodeset && len(args[0].ns) > 0:
			n = args[0].ns[0]
		}
		if n == nil {
			return strVal(""), nil
		}
		return strVal(n.name), nil
	default:
		return xval{}, fmt.Errorf("unsupported function %q", x.name)
	}
}

// xpathRound implements XPath round(): round half towards positive infinity.
func xpathRound(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	return math.Floor(x + 0.5)
}

func xpathSubstring(args []xval) string {
	rs := []rune(args[0].toStr())
	from := xpathRound(args[1].toNum())
	to := math.Inf(1)
	if len(args) == 3 {
		to = from + xpathRound(args[2].toNum())
	}
	var b strings.Builder
	for i, r := range rs {
		pos := float64(i + 1)
		if pos >= from && pos < to {
			b.WriteRune(r)
		}
	}
	return b.String()
}
