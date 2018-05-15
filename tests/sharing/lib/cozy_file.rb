# File is already taken by the stdlib
class CozyFile
  include Model
  include Model::Files

  attr_reader :name, :dir_id, :mime, :trashed, :md5sum

  def self.get_id_from_path(inst, path)
    file = CozyFile.find_by_path inst, path
    file.couch_id
  end

  def self.load_from_url(inst, path)
    opts = {
      content_type: :json,
      accept: :json,
      authorization: "Bearer #{inst.token_for doctype}"
    }
    res = inst.client[path].get opts
    j = JSON.parse(res.body)["data"]
    id = j["id"]
    rev = j["rev"]
    j = j["attributes"]
    f = CozyFile.new(
      name: j["name"],
      dir_id: j["dir_id"],
      trashed: j["trashed"],
      md5sum: j["md5sum"]
    )
    f.couch_id = id
    f.couch_rev = rev
    f
  end

  def self.find_by_path(inst, path)
    load_from_url inst, "/files/metadata?Path=#{path}"
  end

  def self.find(inst, id)
    load_from_url inst, "/files/#{id}"
  end

  def self.options_from_fixture(filename, opts = {})
    opts = opts.dup
    opts[:content] = File.read filename
    opts[:name] ||= "#{Faker::Internet.slug}#{File.extname(filename)}"
    opts[:mime] ||= MimeMagic.by_path filename
    opts
  end

  def self.doctype
    "io.cozy.files"
  end

  def initialize(opts = {})
    @name = opts[:name] || Faker::Internet.slug
    @dir_id = opts[:dir_id] || "io.cozy.files.root-dir"
    @mime = opts[:mime] || "text/plain"
    @content = opts[:content] || "Hello world"
    @trashed = opts[:trashed]
    @md5sum = opts[:md5sum]
  end

  def save(inst)
    opts = {
      accept: "application/vnd.api+json",
      authorization: "Bearer #{inst.token_for doctype}",
      :"content-type" => mime
    }
    res = inst.client["/files/#{dir_id}?Type=file&Name=#{name}"].post @content, opts
    j = JSON.parse(res.body)["data"]
    @couch_id = j["id"]
    @couch_rev = j["meta"]["rev"]
  end

  def overwrite(inst, opts)
    @mime = opts[:mime] || @mime
    @content = opts[:content] || "New content"
    opts = {
      accept: "application/vnd.api+json",
      authorization: "Bearer #{inst.token_for doctype}",
      :"content-type" => mime
    }
    res = inst.client["/files/#{@couch_id}"].put @content, opts
    j = JSON.parse(res.body)["data"]
    @couch_rev = j["meta"]["rev"]
    @md5sum = j["attributes"]["md5sum"]
  end

  PHOTOS = %w(about apps architecture business community faq features try).freeze

  def self.create_photos(inst, opts = {})
    dir = File.expand_path "../.photos", Helpers.current_dir
    PHOTOS.map do |photo|
      create inst, options_from_fixture("#{dir}/#{photo}.jpg", opts)
    end
  end

  def self.ensure_photos_in_cache
    dir = File.expand_path "../.photos", Helpers.current_dir
    return if Dir.exist? dir
    FileUtils.mkdir_p dir
    PHOTOS.each do |photo|
      url = "https://cozy.io/fr/images/bkg-#{photo}.jpg"
      `wget -q #{url} -O #{dir}/#{photo}.jpg`
    end
  end
end
