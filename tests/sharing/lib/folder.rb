class Folder
  ROOT_DIR = "io.cozy.files.root-dir".freeze
  TRASH_DIR = "io.cozy.files.trash-dir".freeze
  NO_LONGER_SHARED_DIR = "io.cozy.files.no-longer-shared-dir".freeze

  include Model::Files

  attr_reader :name, :dir_id, :children, :path, :restore_path

  def self.load_from_url(inst, path)
    opts = {
      content_type: :json,
      accept: :json,
      authorization: "Bearer #{inst.token_for doctype}"
    }
    res = inst.client[path].get opts
    j = JSON.parse(res.body)["data"]
    id = j["id"]
    rev = j.dig "meta", "rev"
    j = j["attributes"]
    f = Folder.new(
      name: j["name"],
      dir_id: j["dir_id"],
      path: j["path"],
      restore_path: j["restore_path"]
    )
    f.couch_id = id
    f.couch_rev = rev
    f
  end

  def self.children(inst, path)
    path = "/files/metadata?Path=#{path}"
    opts = {
      content_type: :json,
      accept: :json,
      authorization: "Bearer #{inst.token_for doctype}"
    }
    res = inst.client[path].get opts
    j = JSON.parse(res.body)["included"]

    dirs = []
    files = []
    (j || []).map do |child|
      id = child["id"]
      rev = child.dig "meta", "rev"
      child = child["attributes"]
      type = child["type"]
      if type == "directory"
        f = Folder.new(
          name: child["name"],
          dir_id: child["dir_id"],
          path: child["path"],
          restore_path: child["restore_path"]
        )
        dirs << f
      else
        f = CozyFile.new(
          name: child["name"],
          dir_id: child["dir_id"],
          trashed: child["trashed"],
          md5sum: child["md5sum"],
          size: child["size"],
          executable: child["executable"],
          file_class: child["class"],
          metadata: child["metadata"]
        )
        files << f
      end
      f.couch_id = id
      f.couch_rev = rev
    end
    [dirs, files]
  end

  def restore_from_trash(inst)
    opts = {
      authorization: "Bearer #{inst.token_for doctype}"
    }
    res = inst.client["/files/trash/#{@couch_id}"].post nil, opts
    j = JSON.parse(res.body)["data"]
    @restore_path = nil
    @path = j.dig "attributes", "path"
    @dir_id = j.dig "attributes", "dir_id"
  end

  def initialize(opts = {})
    @name = opts[:name] || Faker::Internet.slug
    @dir_id = opts[:dir_id] || ROOT_DIR
    @path = opts[:path] || "/#{@name}"
    @restore_path = opts[:restore_path]
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
