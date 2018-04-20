module Model
  module ClassMethods
    def create(inst, opts = {})
      obj = new opts
      obj.save inst
      obj
    end

    def find(inst, id)
      ap "find #{inst} #{id}"
    end
  end

  def to_json
    JSON.generate as_json
  end

  def save(inst)
    opts = {
      content_type: :json,
      accept: :json,
      authorization: "Bearer #{inst.token_for doctype}"
    }
    res = if couch_id
            inst.client["/data/#{doctype}/#{couch_id}"].put to_json, opts
          else
            inst.client["/data/#{doctype}/"].post to_json, opts
          end
    j = JSON.parse(res.body)
    @couch_id = j["id"]
    @couch_rev = j["rev"]
  end

  def self.included(klass)
    klass.extend ClassMethods
    klass.send :attr_accessor, :couch_id, :couch_rev
  end
end
