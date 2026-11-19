package cwquery

// Query is the top-level parse result: discriminated by Kind into either a
// SELECT-form metric query (tiers 1/2/4 - simple comparison, metric math,
// anomaly detection) or an ALARM/AND/OR/NOT composite expression (tier 3).
type Query struct {
	Kind      string // "metric" | "composite"
	Metric    *MetricQuery
	Composite CompositeExpr
}

// MetricQuery is the parsed SELECT ... FROM ... WHERE ... HAVING ... form.
type MetricQuery struct {
	Select    SelectClause
	Namespace string
	Where     []DimPredicate
	Having    HavingClause
}

// SelectClause holds exactly one of BareMetric, StatCall, or MathExpr,
// enforced by how the parser builds it (see parseSelectList) rather than
// by the Go type system.
type SelectClause struct {
	BareMetric *string
	StatCall   *StatCallExpr
	MathExpr   Expr
	Alias      string // "" unless MathExpr is set - tier 2 always requires AS
}

// StatCallExpr is a tier-1 stat-function call, e.g. p99(Latency).
type StatCallExpr struct {
	Stat   string
	Metric string
}

// DimPredicate is one `dimension = 'value'` entry from a WHERE clause.
type DimPredicate struct {
	Dimension string
	Value     string
}

// HavingClause holds exactly one of Threshold or Band, matching the
// grammar's having_clause production.
type HavingClause struct {
	ValueRef  string
	Threshold *ThresholdPred
	Band      *BandPred
}

// ThresholdPred is a plain `value_ref <comparator> N` predicate.
// Comparator is whatever token the parser accepted, including "=" and
// "!=" - Compile rejects those two, since CloudWatch has no real equality
// comparison_operator; the grammar itself stays permissive so that mistake
// produces a clear semantic error instead of a confusing syntax error.
type ThresholdPred struct {
	Comparator string
	Value      float64
}

// BandPred is an anomaly-detection predicate: `value_ref OUTSIDE BAND(n)`.
type BandPred struct {
	Direction string // "OUTSIDE" | "ABOVE" | "BELOW"
	N         float64
}

// Expr is a node in a tier-2 metric-math expression tree.
type Expr interface{ isExpr() }

type NumberLit struct{ Value float64 }
type MetricRef struct{ Name string }
type BinaryExpr struct {
	Op          string // "+" | "-" | "*" | "/"
	Left, Right Expr
}

func (NumberLit) isExpr()  {}
func (MetricRef) isExpr()  {}
func (BinaryExpr) isExpr() {}

// CompositeExpr is a node in a tier-3 composite boolean expression tree.
type CompositeExpr interface{ isCompositeExpr() }

type AlarmRef struct{ Name string }
type NotExpr struct{ X CompositeExpr }
type AndExpr struct{ Left, Right CompositeExpr }
type OrExpr struct{ Left, Right CompositeExpr }

func (AlarmRef) isCompositeExpr() {}
func (NotExpr) isCompositeExpr()  {}
func (AndExpr) isCompositeExpr()  {}
func (OrExpr) isCompositeExpr()   {}
