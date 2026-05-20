package schema

// LifecycleProto is the raw .proto schema content for the submission.lifecycle topic.
// All services that produce or consume lifecycle events register this schema with
// the Schema Registry on startup. The registry is idempotent — re-registering the
// same schema returns the existing ID.
const LifecycleProto = `syntax = "proto3";
package obarena.v1;
message LifecycleEvent {
  oneof event {
    SubmissionCreated submission_created = 1;
    BuildComplete build_complete = 2;
    BuildFailed build_failed = 3;
    SandboxReady sandbox_ready = 4;
    SandboxFailed sandbox_failed = 5;
    TestComplete test_complete = 6;
  }
}
message SubmissionCreated {
  string submission_id = 1;
  string language = 2;
  string team_name = 3;
  string artifact_path = 4;
  int64 created_at = 5;
}
message BuildComplete {
  string submission_id = 1;
  string binary_path = 2;
  string language = 3;
  string team_name = 4;
  int64 built_at = 5;
}
message BuildFailed {
  string submission_id = 1;
  string reason = 2;
  int64 failed_at = 3;
}
message SandboxReady {
  string submission_id = 1;
  string pod_name = 2;
  string pod_ip = 3;
  int32 http_port = 4;
  int32 ws_port = 5;
  string team_name = 6;
  int64 ready_at = 7;
}
message SandboxFailed {
  string submission_id = 1;
  string reason = 2;
  int64 failed_at = 3;
}
message TestComplete {
  string submission_id = 1;
  string team_name = 2;
  bool success = 3;
  string reason = 4;
  int64 completed_at = 5;
}
`

// MetricsProto is the raw .proto schema content for the bot.metrics topic.
const MetricsProto = `syntax = "proto3";
package obarena.v1;
message BotMetrics {
  string team_name = 1;
  string submission_id = 2;
  string test_run_id = 3;
  int64 duration_ms = 4;
  int64 orders_sent = 5;
  int64 acks_recv = 6;
  int64 fills_recv = 7;
  int64 rejects_recv = 8;
  int64 stale_orders = 9;
  int64 ack_p50_us = 10;
  int64 ack_p90_us = 11;
  int64 ack_p99_us = 12;
  int64 ack_p999_us = 13;
  int64 ack_max_us = 14;
  int64 fill_p50_us = 15;
  int64 fill_p90_us = 16;
  int64 fill_p99_us = 17;
  int64 fill_p999_us = 18;
  int64 fill_max_us = 19;
  double tps = 20;
  double correctness_score = 21;
  int64 emitted_at = 22;
}
`
