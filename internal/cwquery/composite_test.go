package cwquery

import "testing"

func TestParseCompositeAnd(t *testing.T) {
	query := `ALARM("nat-a-error-port-allocation") AND ALARM("nat-a-packets-drop")`

	got, err := Parse(query)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.Kind != "composite" {
		t.Fatalf("Kind = %q, want %q", got.Kind, "composite")
	}
	want := `ALARM("nat-a-error-port-allocation") AND ALARM("nat-a-packets-drop")`
	if got.AlarmRule != want {
		t.Fatalf("AlarmRule = %q, want %q", got.AlarmRule, want)
	}
}

func TestParseCompositeOrAndNotWithParens(t *testing.T) {
	query := `(ALARM("cpu-high") OR ALARM("memory-high")) AND NOT ALARM("maintenance-window")`

	got, err := Parse(query)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	want := `(ALARM("cpu-high") OR ALARM("memory-high")) AND NOT ALARM("maintenance-window")`
	if got.AlarmRule != want {
		t.Fatalf("AlarmRule = %q, want %q", got.AlarmRule, want)
	}
}

func TestParseCompositePrecedenceNoRedundantParens(t *testing.T) {
	// AND binds tighter than OR, so this should round-trip with no parens
	// at all - the pretty-printer must not over-parenthesize.
	query := `ALARM("a") AND ALARM("b") OR ALARM("c") AND ALARM("d")`

	got, err := Parse(query)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	want := `ALARM("a") AND ALARM("b") OR ALARM("c") AND ALARM("d")`
	if got.AlarmRule != want {
		t.Fatalf("AlarmRule = %q, want %q", got.AlarmRule, want)
	}
}

func TestParseCompositeDoubleNot(t *testing.T) {
	query := `NOT NOT ALARM("x")`

	got, err := Parse(query)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	want := `NOT NOT ALARM("x")`
	if got.AlarmRule != want {
		t.Fatalf("AlarmRule = %q, want %q", got.AlarmRule, want)
	}
}
