# Outboxテーブル。ローカル環境ではリレーが読み、stgではDMSがbinlog経由で読む
class OutboxEvent < ApplicationRecord
  self.table_name = "outbox"

  validates :event_id, :aggregate_id, :event_type, :payload, presence: true

  scope :unpublished_in_order, -> { where(published: false).order(:id) }

  def self.mark_published!(ids)
    return if ids.empty?

    where(id: ids).update_all(published: true)
  end
end
