class Nextcloud
  def initialize(inst, account_id)
    @inst = inst
    @token = @inst.token_for "io.cozy.files"
    @base_path = "/remote/nextcloud/#{account_id}"
  end

  def list(path)
    opts = {
      accept: :json,
      authorization: "Bearer #{@token}"
    }
    res = @inst.client["#{@base_path}#{encode_path path}"].get opts
    JSON.parse(res.body)
  end

  def mkdir(path)
    opts = {
      accept: :json,
      authorization: "Bearer #{@token}"
    }
    @inst.client["#{@base_path}#{encode_path path}?Type=directory"].put nil, opts
  end

  def upload(path, filename)
    mime = MiniMime.lookup_by_filename(filename).content_type
    ap mime
    content = File.read filename
    opts = {
      accept: :json,
      authorization: "Bearer #{@token}",
      :"content-type" => mime
    }
    @inst.client["#{@base_path}#{encode_path path}?Type=file"].put content, opts
  end

  def encode_path(path)
    path.split("/").map { |s| ERB::Util.url_encode s }.join("/")
  end
end
