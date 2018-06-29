class Album
  include Model

  attr_reader :name, :created_at

  def self.doctype
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

  def remove_photo(inst, photo)
    opts = {
      accept: "application/vnd.api+json",
      authorization: "Bearer #{inst.token_for [doctype, photo.doctype]}",
      :"content-type" => "application/vnd.api+json"
    }
    body = JSON.generate data: [{ type: doctype, id: @couch_id }]
    url = "#{inst.domain}/files/#{photo.couch_id}/relationships/referenced_by"
    RestClient::Request.execute(method: :delete, url: url, payload: body, headers: opts)
  end

  def self.find(inst, id)
    opts = {
      content_type: :json,
      accept: :json,
      authorization: "Bearer #{inst.token_for doctype}"
    }
    res = inst.client["/data/#{doctype}/#{id}"].get opts
    j = JSON.parse(res.body)
    album = Album.new(
      name: j["name"],
      created_at: j["created_at"]
    )
    album.couch_id = j["_id"]
    album.couch_rev = j["_rev"]
    album
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
