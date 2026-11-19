package cwquery

import (
	"reflect"
	"testing"
)

func TestParseMetricMath(t *testing.T) {
	query := `SELECT (BytesOutToDestination + BytesInFromDestination) * 8 / 300 / 1e11 * 100 AS utilization
FROM AWS/NATGateway
WHERE NatGatewayId = 'nat-123'
HAVING utilization > 80`

	got, err := Parse(query)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	want := &Compiled{
		Kind:               "metric",
		ComparisonOperator: "GreaterThanThreshold",
		Threshold:          f64p(80),
		Metrics: []MetricEntry{
			{ID: "m1", Namespace: "AWS/NATGateway", MetricName: "BytesOutToDestination", Stat: "Sum", Dimensions: map[string]string{"NatGatewayId": "nat-123"}},
			{ID: "m2", Namespace: "AWS/NATGateway", MetricName: "BytesInFromDestination", Stat: "Sum", Dimensions: map[string]string{"NatGatewayId": "nat-123"}},
			{ID: "m3", Expression: "(((((m1 + m2) * 8) / 300) / 1e+11) * 100)", ReturnData: boolp(true)},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() = %#v, want %#v", got, want)
	}
}

func TestParseMetricMathSingleMetric(t *testing.T) {
	// Not every tier-2 query needs more than one distinct metric - a bare
	// math expression around a single reference is still legal, it just
	// produces one raw entry plus one expression entry.
	query := `SELECT BytesOutToDestination * 8 AS bits
FROM AWS/NATGateway
WHERE NatGatewayId = 'nat-123'
HAVING bits > 1000`

	got, err := Parse(query)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(got.Metrics) != 2 {
		t.Fatalf("got %d metric entries, want 2", len(got.Metrics))
	}
	if got.Metrics[1].Expression != "(m1 * 8)" {
		t.Fatalf("expression = %q, want %q", got.Metrics[1].Expression, "(m1 * 8)")
	}
}

func TestParseHavingAliasMismatch(t *testing.T) {
	query := `SELECT (BytesOutToDestination + BytesInFromDestination) * 8 AS utilization
FROM AWS/NATGateway
WHERE NatGatewayId = 'nat-123'
HAVING wrong_name > 80`

	if _, err := Parse(query); err == nil {
		t.Fatal("expected an error for a HAVING clause referencing the wrong alias, got nil")
	}
}
