terraform {
  required_providers {
    cwquery = {
      source = "rafalmasiarek/cwquery"
    }
  }
}

provider "cwquery" {}

# Tier 1: simple comparison.
output "simple" {
  value = provider::cwquery::parse(<<-EOT
    SELECT ErrorPortAllocation FROM AWS/NATGateway
    WHERE NatGatewayId = 'nat-0123456789abcdef0'
    HAVING value > 0
  EOT
  )
}

# Tier 1: stat/aggregation (percentile).
output "stat_call" {
  value = provider::cwquery::parse(<<-EOT
    SELECT p99(Latency) FROM AWS/ApplicationELB
    WHERE LoadBalancer = 'app/my-alb/abc123'
    HAVING value > 500
  EOT
  )
}

# Tier 2: metric math.
output "metric_math" {
  value = provider::cwquery::parse(<<-EOT
    SELECT (BytesOutToDestination + BytesInFromDestination) * 8 / 300 / 1e11 * 100 AS utilization
    FROM AWS/NATGateway
    WHERE NatGatewayId = 'nat-123'
    HAVING utilization > 80
  EOT
  )
}

# Tier 3: composite (AND/OR/NOT over existing alarms).
output "composite" {
  value = provider::cwquery::parse(
    "(ALARM(\"cpu-high\") OR ALARM(\"memory-high\")) AND NOT ALARM(\"maintenance-window\")"
  )
}

# Tier 4: anomaly detection.
output "anomaly" {
  value = provider::cwquery::parse(<<-EOT
    SELECT CPUUtilization FROM AWS/EC2
    WHERE AutoScalingGroupName = 'my-asg'
    HAVING value OUTSIDE BAND(2)
  EOT
  )
}
