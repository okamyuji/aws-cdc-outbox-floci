namespace :outbox do
  desc "outboxをポーリングしてKinesisへ中継する(ローカル環境専用)"
  task relay: :environment do
    stream_name = ENV.fetch("KINESIS_STREAM_NAME")
    interval_ms = ENV.fetch("POLL_INTERVAL_MS", "500").to_i
    relay = OutboxRelay.new(kinesis: Aws::Kinesis::Client.new, stream_name: stream_name)

    # SIGTERM/SIGINTで現在のバッチ処理を終えてから停止する(docker stopで途中破棄しない)
    stopping = false
    [ "TERM", "INT" ].each { |sig| Signal.trap(sig) { stopping = true } }

    Rails.logger.info("リレーを起動します stream=#{stream_name} interval=#{interval_ms}ms")
    consecutive_failures = 0
    until stopping
      begin
        count = relay.relay_once
        consecutive_failures = 0
        Rails.logger.info("outboxイベントを中継しました count=#{count}") if count.positive?
      rescue => e
        consecutive_failures += 1
        Rails.logger.error("中継に失敗しました。次のポーリングで再試行します: #{e.message.inspect}")
      end
      # 連続失敗時は最大10秒まで指数的に間隔を広げ、障害中のログ洪水を防ぐ
      backoff = [ interval_ms * (2**[ consecutive_failures, 5 ].min), 10_000 ].min
      sleep(backoff / 1000.0)
    end
    Rails.logger.info("リレーを停止します")
  end
end
