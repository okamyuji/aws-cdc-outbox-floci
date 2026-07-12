namespace :outbox do
  desc "outboxをポーリングしてKinesisへ中継する(ローカル環境専用)"
  task relay: :environment do
    stream_name = ENV.fetch("KINESIS_STREAM_NAME")
    interval_ms = ENV.fetch("POLL_INTERVAL_MS", "500").to_i
    relay = OutboxRelay.new(kinesis: Aws::Kinesis::Client.new, stream_name: stream_name)

    Rails.logger.info("リレーを起動します stream=#{stream_name} interval=#{interval_ms}ms")
    loop do
      begin
        count = relay.relay_once
        Rails.logger.info("outboxイベントを中継しました count=#{count}") if count.positive?
      rescue => e
        Rails.logger.error("中継に失敗しました。次のポーリングで再試行します: #{e.message}")
      end
      sleep(interval_ms / 1000.0)
    end
  end
end
