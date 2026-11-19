package cwquery

import "testing"

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name  string
		query string
	}{
		{"missing FROM", `SELECT CPUUtilization WHERE AutoScalingGroupName = 'x' HAVING value > 1`},
		{"equality comparator rejected", `SELECT CPUUtilization FROM AWS/EC2 WHERE AutoScalingGroupName = 'x' HAVING value = 80`},
		{"inequality comparator rejected", `SELECT CPUUtilization FROM AWS/EC2 WHERE AutoScalingGroupName = 'x' HAVING value != 80`},
		{"unbalanced paren in math expr", `SELECT (CPUUtilization + 1 AS x FROM AWS/EC2 WHERE AutoScalingGroupName = 'x' HAVING x > 1`},
		{"unbalanced paren in composite", `(ALARM("a") AND ALARM("b")`},
		{"unterminated string", `SELECT CPUUtilization FROM AWS/EC2 WHERE AutoScalingGroupName = 'x HAVING value > 1`},
		{"empty input", ``},
		{"garbage first token", `WHATEVER THIS IS NOT VALID`},
		{"trailing garbage after composite", `ALARM("a") AND ALARM("b") BOGUS`},
		{"trailing garbage after select", `SELECT CPUUtilization FROM AWS/EC2 WHERE AutoScalingGroupName = 'x' HAVING value > 1 BOGUS`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Parse(c.query); err == nil {
				t.Fatalf("Parse(%q) succeeded, want an error", c.query)
			}
		})
	}
}
