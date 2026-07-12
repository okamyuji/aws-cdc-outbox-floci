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

# APIサーバー側のAUTH_TOKENとdelivery Lambda側のTARGET_API_TOKENの単一ソース。
# 起動時は terraform output -raw target_api_token から取得し、二重管理をしない
output "target_api_token" {
  description = "ターゲットAPIのBearerトークン(単一ソース)"
  value       = var.target_api_token
  sensitive   = true
}
