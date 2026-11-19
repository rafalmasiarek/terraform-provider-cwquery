package cwquery

import (
	"fmt"
	"strings"
)

// Compiled is the structured result of parsing and semantically validating
// a cwquery string. This exact shape is also the contract with the
// Terraform-facing `parse` function and, through it, with
// terraform-aws-cloudwatch's metric-alarm/composite-alarm modules - don't
// change field names/types here without updating both.
type Compiled struct {
	Kind               string // "metric" | "composite"
	ComparisonOperator string
	Threshold          *float64
	ThresholdMetricID  *string
	Metrics            []MetricEntry
	AlarmRule          string
}

// MetricEntry mirrors terraform-aws-cloudwatch/modules/metric-alarm's
// `metrics` variable object shape field-for-field, so the Terraform
// function's output can be assigned to that variable with no transform.
type MetricEntry struct {
	ID         string
	Namespace  string
	MetricName string
	Stat       string
	Unit       string
	Dimensions map[string]string
	Expression string
	ReturnData *bool
}

// basicStats are the only values CloudWatch's legacy top-level `statistic`
// attribute accepts - a percentile (e.g. "p99") requires the
// metric_query.metric.stat path instead (verified against AWS SDK docs).
var basicStats = map[string]bool{
	"SampleCount": true, "Average": true, "Sum": true, "Minimum": true, "Maximum": true,
}

// Compile lowers a parsed Query into a Compiled result, applying the
// semantic rules the grammar alone can't express: comparator legality,
// value-reference/alias agreement, metric-math id assignment, anomaly-band
// construction, and alarm_rule pretty-printing.
func Compile(q *Query) (*Compiled, error) {
	if q.Kind == "composite" {
		rule, err := renderAlarmRule(q.Composite)
		if err != nil {
			return nil, err
		}
		return &Compiled{Kind: "composite", AlarmRule: rule}, nil
	}
	return compileMetricQuery(q.Metric)
}

func compileMetricQuery(mq *MetricQuery) (*Compiled, error) {
	dims := map[string]string{}
	for _, d := range mq.Where {
		dims[d.Dimension] = d.Value
	}

	expectedRef := "value"
	if mq.Select.Alias != "" {
		expectedRef = mq.Select.Alias
	}
	if !strings.EqualFold(mq.Having.ValueRef, expectedRef) {
		return nil, fmt.Errorf("HAVING references %q, but the SELECT clause produces %q", mq.Having.ValueRef, expectedRef)
	}

	switch {
	case mq.Select.BareMetric != nil || mq.Select.StatCall != nil:
		return compileSimpleSelect(mq, dims)
	case mq.Select.MathExpr != nil:
		return compileMathSelect(mq, dims)
	}
	return nil, fmt.Errorf("internal error: SELECT clause has no metric, stat-call, or math expression")
}

func compileSimpleSelect(mq *MetricQuery, dims map[string]string) (*Compiled, error) {
	var metricName, stat string
	if mq.Select.BareMetric != nil {
		metricName = *mq.Select.BareMetric
		stat = "Sum"
	} else {
		metricName = mq.Select.StatCall.Metric
		stat = mq.Select.StatCall.Stat
	}

	entry := MetricEntry{
		Namespace:  mq.Namespace,
		MetricName: metricName,
		Stat:       stat,
		Dimensions: dims,
	}
	// A percentile stat can't go through the top-level `statistic`
	// attribute - only metric_query.metric.stat accepts it. Assigning an
	// id even for this lone entry forces the consuming module down the
	// metric_query rendering path instead of the plain-attribute path.
	if !basicStats[stat] {
		entry.ID = "m1"
	}

	if mq.Having.Band != nil {
		return compileAnomalyBand(entry, mq.Having.Band)
	}

	if err := requireRealComparator(mq.Having.Threshold); err != nil {
		return nil, err
	}
	threshold := mq.Having.Threshold.Value
	return &Compiled{
		Kind:               "metric",
		ComparisonOperator: comparatorToOperator(mq.Having.Threshold.Comparator),
		Threshold:          &threshold,
		Metrics:            []MetricEntry{entry},
	}, nil
}

func compileAnomalyBand(raw MetricEntry, band *BandPred) (*Compiled, error) {
	if raw.ID == "" {
		raw.ID = "m1"
	}
	returnFalse := false
	raw.ReturnData = &returnFalse

	bandID := "ad1"
	returnTrue := true
	bandEntry := MetricEntry{
		ID:         bandID,
		Expression: fmt.Sprintf("ANOMALY_DETECTION_BAND(%s, %s)", raw.ID, trimFloat(band.N)),
		ReturnData: &returnTrue,
	}

	var op string
	switch band.Direction {
	case "OUTSIDE":
		op = "LessThanLowerOrGreaterThanUpperThreshold"
	case "ABOVE":
		op = "GreaterThanUpperThreshold"
	case "BELOW":
		op = "LessThanLowerThreshold"
	default:
		return nil, fmt.Errorf("internal error: unknown band direction %q", band.Direction)
	}

	return &Compiled{
		Kind:               "metric",
		ComparisonOperator: op,
		ThresholdMetricID:  &bandID,
		Metrics:            []MetricEntry{raw, bandEntry},
	}, nil
}

func compileMathSelect(mq *MetricQuery, dims map[string]string) (*Compiled, error) {
	if mq.Having.Band != nil {
		return nil, fmt.Errorf("anomaly detection (OUTSIDE/ABOVE/BELOW BAND) is not supported with a metric-math SELECT - it applies to a single raw metric")
	}
	if err := requireRealComparator(mq.Having.Threshold); err != nil {
		return nil, err
	}

	refs, err := collectMetricRefs(mq.Select.MathExpr)
	if err != nil {
		return nil, err
	}

	ids := map[string]string{}
	entries := make([]MetricEntry, 0, len(refs)+1)
	for i, name := range refs {
		id := fmt.Sprintf("m%d", i+1)
		ids[name] = id
		entries = append(entries, MetricEntry{
			ID:         id,
			Namespace:  mq.Namespace,
			MetricName: name,
			Stat:       "Sum",
			Dimensions: dims,
		})
	}

	exprStr, err := renderMathExpr(mq.Select.MathExpr, ids)
	if err != nil {
		return nil, err
	}
	returnTrue := true
	entries = append(entries, MetricEntry{
		ID:         fmt.Sprintf("m%d", len(refs)+1),
		Expression: exprStr,
		ReturnData: &returnTrue,
	})

	threshold := mq.Having.Threshold.Value
	return &Compiled{
		Kind:               "metric",
		ComparisonOperator: comparatorToOperator(mq.Having.Threshold.Comparator),
		Threshold:          &threshold,
		Metrics:            entries,
	}, nil
}

func requireRealComparator(t *ThresholdPred) error {
	if t == nil {
		return nil
	}
	switch t.Comparator {
	case ">", ">=", "<", "<=":
		return nil
	default:
		return fmt.Errorf("comparator %q has no equivalent CloudWatch comparison_operator - only >, >=, <, <= are supported", t.Comparator)
	}
}

func comparatorToOperator(c string) string {
	switch c {
	case ">":
		return "GreaterThanThreshold"
	case ">=":
		return "GreaterThanOrEqualToThreshold"
	case "<":
		return "LessThanThreshold"
	case "<=":
		return "LessThanOrEqualToThreshold"
	}
	return ""
}

// collectMetricRefs walks a math Expr and returns the distinct metric
// names referenced, in first-occurrence order - this order drives
// deterministic id assignment (m1, m2, ...).
func collectMetricRefs(e Expr) ([]string, error) {
	seen := map[string]bool{}
	var order []string
	var walk func(Expr) error
	walk = func(e Expr) error {
		switch n := e.(type) {
		case MetricRef:
			if !seen[n.Name] {
				seen[n.Name] = true
				order = append(order, n.Name)
			}
		case NumberLit:
			// nothing to collect
		case BinaryExpr:
			if err := walk(n.Left); err != nil {
				return err
			}
			if err := walk(n.Right); err != nil {
				return err
			}
		default:
			return fmt.Errorf("internal error: unknown math expr node %T", e)
		}
		return nil
	}
	if err := walk(e); err != nil {
		return nil, err
	}
	if len(order) == 0 {
		return nil, fmt.Errorf("metric math expression references no metrics")
	}
	return order, nil
}

// renderMathExpr re-serializes a math Expr into AWS metric-math syntax,
// substituting each bare metric name for its assigned metric_query id.
// This is a pretty-print from the AST, never a byte passthrough of the
// original query text.
func renderMathExpr(e Expr, ids map[string]string) (string, error) {
	switch n := e.(type) {
	case NumberLit:
		return trimFloat(n.Value), nil
	case MetricRef:
		id, ok := ids[n.Name]
		if !ok {
			return "", fmt.Errorf("internal error: no id assigned for metric %q", n.Name)
		}
		return id, nil
	case BinaryExpr:
		left, err := renderMathExpr(n.Left, ids)
		if err != nil {
			return "", err
		}
		right, err := renderMathExpr(n.Right, ids)
		if err != nil {
			return "", err
		}
		// Parenthesize every binary node unconditionally - simplest correct
		// approach for generated (not hand-written) output; not worth a
		// precedence-aware minimal-parens printer for marginal readability.
		return fmt.Sprintf("(%s %s %s)", left, n.Op, right), nil
	}
	return "", fmt.Errorf("internal error: unknown math expr node %T", e)
}

func trimFloat(v float64) string {
	return fmt.Sprintf("%g", v)
}

// renderAlarmRule re-serializes a CompositeExpr into AWS's real alarm_rule
// syntax: ALARM("name") combined with AND/OR/NOT, with parens only where
// precedence requires them.
func renderAlarmRule(e CompositeExpr) (string, error) {
	s, _, err := renderComposite(e)
	return s, err
}

// renderComposite returns the rendered string and its precedence tier
// (0 = atom/NOT, 1 = AND, 2 = OR) so the caller can decide whether a child
// needs parenthesizing - AND binds tighter than OR, matching the grammar.
func renderComposite(e CompositeExpr) (string, int, error) {
	switch n := e.(type) {
	case AlarmRef:
		return fmt.Sprintf("ALARM(%q)", n.Name), 0, nil
	case NotExpr:
		inner, prec, err := renderComposite(n.X)
		if err != nil {
			return "", 0, err
		}
		if prec > 0 {
			inner = "(" + inner + ")"
		}
		return "NOT " + inner, 0, nil
	case AndExpr:
		left, lp, err := renderComposite(n.Left)
		if err != nil {
			return "", 0, err
		}
		right, rp, err := renderComposite(n.Right)
		if err != nil {
			return "", 0, err
		}
		if lp > 1 {
			left = "(" + left + ")"
		}
		if rp > 1 {
			right = "(" + right + ")"
		}
		return left + " AND " + right, 1, nil
	case OrExpr:
		left, lp, err := renderComposite(n.Left)
		if err != nil {
			return "", 0, err
		}
		right, rp, err := renderComposite(n.Right)
		if err != nil {
			return "", 0, err
		}
		if lp > 2 {
			left = "(" + left + ")"
		}
		if rp > 2 {
			right = "(" + right + ")"
		}
		return left + " OR " + right, 2, nil
	}
	return "", 0, fmt.Errorf("internal error: unknown composite expr node %T", e)
}
