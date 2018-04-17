class Folder
  attr_accessor :couch_id
  attr_reader :name, :dir_id

  def doctype
    "io.cozy.files"
  end

  def initialize(opts = {})
    @name = opts[:name] || Faker::Internet.slug
    @dir_id = opts[:dir_id] || "io.cozy.files.root-dir"
  end
end
