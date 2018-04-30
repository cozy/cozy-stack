class Folder
  include Model
  include Model::Files
  attr_accessor :couch_id, :couch_rev
  attr_reader :name, :dir_id, :children

  def find_by_name(inst, path)
    opts = {
      content_type: :json,
      accept: :json,
      authorization: "Bearer #{inst.token_for doctype}"
    }
    res = inst.client["/files/metadata?Path=#{path}"].get opts
    j = JSON.parse(res.body)["data"]
    id = j["id"]
    rev = j["rev"]
    j = j["attributes"]
    f = Folder.new(name: j["name"], dir_id: j["dir_id"])
    f.couch_id = id
    f.couch_rev = rev
    f
  end

  def find(inst, id)
    opts = {
      content_type: :json,
      accept: :json,
      authorization: "Bearer #{inst.token_for doctype}"
    }
    res = inst.client["/files/#{id}"].get opts
    j = JSON.parse(res.body)["data"]
    id = j["id"]
    rev = j["rev"]
    j = j["attributes"]
    f = Folder.new(name: j["name"], dir_id: j["dir_id"])
    f.couch_id = id
    f.couch_rev = rev
    f
  end

  def doctype
    "io.cozy.files"
  end

  def initialize(opts = {})
    @name = opts[:name] || Faker::Internet.slug
    @dir_id = opts[:dir_id] || "io.cozy.files.root-dir"
    @children = []
  end

  def save(inst)
    opts = {
      accept: "application/vnd.api+json",
      authorization: "Bearer #{inst.token_for doctype}"
    }
    res = inst.client["/files/#{dir_id}?Type=directory&Name=#{name}"].post nil, opts
    j = JSON.parse(res.body)["data"]
    @couch_id = j["id"]
    @couch_rev = j["rev"]
  end
end
