class Trigger
  include Model

  attr_reader :attributes, :links

  def self.doctype
    "io.cozy.triggers"
  end

  def initialize(opts = {})
    @attributes = opts
  end

  def save(inst)
    opts = {
      content_type: :json,
      accept: :json,
      authorization: "Bearer #{inst.token_for doctype}"
    }
    res = inst.client["/jobs/triggers"].post to_json, opts
    j = JSON.parse(res.body)
    @couch_id = j["data"]["id"]
    @links = j["data"]["links"]
  end

  def as_json
    { data: { attributes: @attributes } }
  end

  def self.from_json(j)
    Trigger.new j
  end

  module Webhook
    def self.create(inst, message, debounce = nil)
      attrs = {
        type: "@webhook",
        worker: "konnector",
        message: message
      }
      attrs[:debounce] = debounce if debounce
      t = Trigger.create inst, attrs
      t.links["webhook"]
    end
  end
end
