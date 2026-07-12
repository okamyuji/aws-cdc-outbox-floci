# べき等性担保テーブル。処理済みイベントIDを記録する
class ProcessedEvent < ApplicationRecord
  self.primary_key = "event_id"
end
