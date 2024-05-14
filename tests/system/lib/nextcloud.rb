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

  def encode_path(path)
    path.split("/").map { |s| ERB::Util.url_encode s }.join("/")
  end
end
