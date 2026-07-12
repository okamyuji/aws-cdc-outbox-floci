output "kinesis_stream_name" {
  description = "CDCイベントストリーム名"
  value       = aws_kinesis_stream.cdc.name
}

output "kinesis_stream_arn" {
  description = "CDCイベントストリームARN"
  value       = aws_kinesis_stream.cdc.arn
}

output "delivery_queue_url" {
  description = "配送用FIFOキューURL"
  value       = aws_sqs_queue.delivery.url
}

output "delivery_dlq_url" {
  description = "配送用DLQのURL"
  value       = aws_sqs_queue.delivery_dlq.url
}
