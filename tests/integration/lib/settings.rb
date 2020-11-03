class Settings
  def self.doctype
    "io.cozy.settings"
  end

  def self.instance(inst)
    opts = {
      accept: :json,
      authorization: "Bearer #{inst.token_for doctype}"
    }
    res = inst.client["/settings/instance"].get opts
    JSON.parse(res).dig "data", "attributes"
  end
end
