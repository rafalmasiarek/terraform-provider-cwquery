// Package cwquery implements a small, hand-written SQL-like query
// language for defining CloudWatch alarms, compiled into the structured
// shape terraform-aws-cloudwatch's metric-alarm/composite-alarm modules
// consume. See docs/grammar-spec.md in this repository for the full
// grammar and worked examples.
package cwquery

// Parse is cwquery's single public entry point: lexes and parses the
// input into an AST (ParseQuery), then compiles it into the structured
// result consumed by the Terraform `parse` function and, through it, by
// terraform-aws-cloudwatch's metric-alarm/composite-alarm modules.
func Parse(input string) (*Compiled, error) {
	q, err := ParseQuery(input)
	if err != nil {
		return nil, err
	}
	return Compile(q)
}
