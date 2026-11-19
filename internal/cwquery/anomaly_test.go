package cwquery

import (
	"reflect"
	"testing"
)

func TestParseAnomalyOutside(t *testing.T) {
	query := `SELECT CPUUtilization FROM AWS/EC2
WHERE AutoScalingGroupName = 'my-asg'
HAVING value OUTSIDE BAND(2)`

	got, err := Parse(query)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	want := &Compiled{
		Kind:               "metric",
		ComparisonOperator: "LessThanLowerOrGreaterThanUpperThreshold",
		ThresholdMetricID:  strp("ad1"),
		Metrics: []MetricEntry{
			{
				ID:         "m1",
				Namespace:  "AWS/EC2",
				MetricName: "CPUUtilization",
				Stat:       "Sum",
				Dimensions: map[string]string{"AutoScalingGroupName": "my-asg"},
				ReturnData: boolp(false),
			},
			{
				ID:         "ad1",
				Expression: "ANOMALY_DETECTION_BAND(m1, 2)",
				ReturnData: boolp(true),
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() = %#v, want %#v", got, want)
	}
}

func TestParseAnomalyDirections(t *testing.T) {
	cases := []struct {
		direction string
		want      string
	}{
		{"OUTSIDE", "LessThanLowerOrGreaterThanUpperThreshold"},
		{"ABOVE", "GreaterThanUpperThreshold"},
		{"BELOW", "LessThanLowerThreshold"},
	}
	for _, c := range cases {
		query := `SELECT CPUUtilization FROM AWS/EC2 WHERE AutoScalingGroupName = 'my-asg' HAVING value ` + c.direction + ` BAND(3)`
		got, err := Parse(query)
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", c.direction, err)
		}
		if got.ComparisonOperator != c.want {
			t.Errorf("direction %q: got operator %q, want %q", c.direction, got.ComparisonOperator, c.want)
		}
		if got.Threshold != nil {
			t.Errorf("direction %q: Threshold should be nil for an anomaly-band alarm, got %v", c.direction, *got.Threshold)
		}
	}
}

func TestParseAnomalyRejectedWithMath(t *testing.T) {
	query := `SELECT (BytesOutToDestination + BytesInFromDestination) * 8 AS bits
FROM AWS/NATGateway
WHERE NatGatewayId = 'nat-123'
HAVING bits OUTSIDE BAND(2)`

	if _, err := Parse(query); err == nil {
		t.Fatal("expected anomaly detection to be rejected for a metric-math SELECT, got nil error")
	}
}
