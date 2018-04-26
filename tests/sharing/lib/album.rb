class Album
  include Model

  attr_reader :name, :created_at

  def doctype
    "io.cozy.photos.albums"
  end

  def initialize(opts = {})
    @name = opts[:name] || Faker::DrWho.quote
    @created_at = opts[:created_at] || Date.today
  end

  def add(inst, photo)
    opts = {
      accept: "application/vnd.api+json",
      authorization: "Bearer #{inst.token_for [doctype, photo.doctype]}",
      :"content-type" => "application/vnd.api+json"
    }
    body = JSON.generate data: [{ type: doctype, id: @couch_id }]
    inst.client["/files/#{photo.couch_id}/relationships/referenced_by"].post body, opts
  end

  def as_json
    {
      name: @name,
      created_at: @created_at.rfc3339
    }
  end

  def as_reference
    {
      doctype: doctype,
      id: @couch_id
    }
  end
end
