package cwquery

import (
	"reflect"
	"testing"
)

func f64p(v float64) *float64 { return &v }
func strp(v string) *string   { return &v }
func boolp(v bool) *bool      { return &v }

func TestParseBareMetric(t *testing.T) {
	query := `SELECT ErrorPortAllocation FROM AWS/NATGateway
WHERE NatGatewayId = 'nat-0123456789abcdef0'
HAVING value > 0`

	got, err := Parse(query)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	want := &Compiled{
		Kind:               "metric",
		ComparisonOperator: "GreaterThanThreshold",
		Threshold:          f64p(0),
		Metrics: []MetricEntry{
			{
				Namespace:  "AWS/NATGateway",
				MetricName: "ErrorPortAllocation",
				Stat:       "Sum",
				Dimensions: map[string]string{"NatGatewayId": "nat-0123456789abcdef0"},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() = %#v, want %#v", got, want)
	}
}

func TestParseStatCall(t *testing.T) {
	query := `SELECT p99(Latency) FROM AWS/ApplicationELB
WHERE LoadBalancer = 'app/my-alb/abc123'
HAVING value > 500`

	got, err := Parse(query)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	want := &Compiled{
		Kind:               "metric",
		ComparisonOperator: "GreaterThanThreshold",
		Threshold:          f64p(500),
		Metrics: []MetricEntry{
			{
				ID:         "m1",
				Namespace:  "AWS/ApplicationELB",
				MetricName: "Latency",
				Stat:       "p99",
				Dimensions: map[string]string{"LoadBalancer": "app/my-alb/abc123"},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() = %#v, want %#v", got, want)
	}
}

func TestParseComparatorVariants(t *testing.T) {
	cases := []struct {
		comparator string
		want       string
	}{
		{">", "GreaterThanThreshold"},
		{">=", "GreaterThanOrEqualToThreshold"},
		{"<", "LessThanThreshold"},
		{"<=", "LessThanOrEqualToThreshold"},
	}
	for _, c := range cases {
		query := `SELECT CPUUtilization FROM AWS/EC2 WHERE AutoScalingGroupName = 'my-asg' HAVING value ` + c.comparator + ` 80`
		got, err := Parse(query)
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", c.comparator, err)
		}
		if got.ComparisonOperator != c.want {
			t.Errorf("comparator %q: got operator %q, want %q", c.comparator, got.ComparisonOperator, c.want)
		}
	}
}

func TestParseMultipleWhereDimensions(t *testing.T) {
	query := `SELECT CPUUtilization FROM AWS/EC2
WHERE AutoScalingGroupName = 'my-asg' AND InstanceId = 'i-123'
HAVING value > 80`

	got, err := Parse(query)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	want := map[string]string{"AutoScalingGroupName": "my-asg", "InstanceId": "i-123"}
	if !reflect.DeepEqual(got.Metrics[0].Dimensions, want) {
		t.Fatalf("Dimensions = %#v, want %#v", got.Metrics[0].Dimensions, want)
	}
}
