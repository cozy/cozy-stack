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

  def move(path, to)
    opts = {
      authorization: "Bearer #{@token}"
    }
    @inst.client["#{@base_path}/move#{encode_path path}?To=#{to}"].post nil, opts
  end

  def copy(path, name)
    opts = {
      authorization: "Bearer #{@token}"
    }
    @inst.client["#{@base_path}/copy#{encode_path path}?Name=#{name}"].post nil, opts
  end

  def mkdir(path)
    opts = {
      authorization: "Bearer #{@token}"
    }
    @inst.client["#{@base_path}#{encode_path path}?Type=directory"].put nil, opts
  end

  def upload(path, filename)
    mime = MiniMime.lookup_by_filename(filename).content_type
    content = File.read filename
    opts = {
      authorization: "Bearer #{@token}",
      :"content-type" => mime
    }
    @inst.client["#{@base_path}#{encode_path path}?Type=file"].put content, opts
  end

  def download(path)
    opts = {
      authorization: "Bearer #{@token}"
    }
    @inst.client["#{@base_path}#{encode_path path}?Dl=1"].get(opts).body
  end

  def upstream(path, from)
    opts = {
      authorization: "Bearer #{@token}"
    }
    @inst.client["#{@base_path}/upstream#{encode_path path}?From=#{from}"].post nil, opts
  end

  def downstream(path, to)
    opts = {
      authorization: "Bearer #{@token}"
    }
    @inst.client["#{@base_path}/downstream#{encode_path path}?To=#{to}"].post nil, opts
  end

  def delete(path)
    opts = {
      authorization: "Bearer #{@token}"
    }
    @inst.client["#{@base_path}#{encode_path path}"].delete opts
  end

  def encode_path(path)
    path.split("/").map { |s| ERB::Util.url_encode s }.join("/")
  end
end
