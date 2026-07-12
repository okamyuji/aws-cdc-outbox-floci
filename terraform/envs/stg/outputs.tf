output "source_cluster_endpoint" {
  description = "ソースAuroraクラスタの書き込みエンドポイント"
  value       = aws_rds_cluster.source.endpoint
}

output "target_cluster_endpoint" {
  description = "ターゲットAuroraクラスタの書き込みエンドポイント"
  value       = aws_rds_cluster.target.endpoint
}

output "kinesis_stream_name" {
  description = "CDCイベントストリーム名"
  value       = module.pipeline.kinesis_stream_name
}

output "delivery_queue_url" {
  description = "配送用FIFOキューURL"
  value       = module.pipeline.delivery_queue_url
}

output "dms_task_arn" {
  description = "DMSレプリケーションタスクARN"
  value       = aws_dms_replication_task.outbox_cdc.replication_task_arn
}
