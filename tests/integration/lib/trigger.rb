class Trigger
  include Model

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
  end

  def as_json
    { data: { attributes: @attributes } }
  end
end
