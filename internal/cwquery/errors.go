package cwquery

import "fmt"

// ParseError is a lexer/parser-level error with a byte-offset position into
// the original query string, so callers (and the Terraform function
// adapter) can report exactly where a query went wrong.
type ParseError struct {
	Pos     int
	Message string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("position %d: %s", e.Pos, e.Message)
}
