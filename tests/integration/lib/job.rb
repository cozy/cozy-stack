class Job
  include Model

  attr_reader :attributes

  def self.doctype
    "io.cozy.jobs"
  end

  def initialize(opts = {})
    @couch_id = opts["id"]
    @attributes = opts["attributes"]
  end

  def done?(inst)
    status(inst) == "done"
  end

  def status(inst)
    opts = {
      accept: 'application/vnd.api+json',
      authorization: "Bearer #{inst.token_for doctype}"
    }
    res = inst.client["/jobs/#{@couch_id}"].get opts
    j = JSON.parse(res.body)
    j.dig("data", "attributes", "state")
  end
end
