class Office
  def self.create(inst, opts = {})
    opts[:name] = opts[:name] || "#{Faker::DrWho.quote}.docx"
    opts[:dir_id] = opts[:dir_id] || Folder::ROOT_DIR
    opts[:mime] = opts[:mime] || "application/msword"
    CozyFile.create inst, opts
  end

  def self.open(inst, id)
    opts = {
      accept: 'application/vnd.api+json',
      authorization: "Bearer #{inst.token_for CozyFile.doctype}"
    }
    res = inst.client["/office/#{id}/open"].get opts
    parsed = JSON.parse(res.body)
    parameters = parsed.dig "data", "attributes"
    parameters["id"] = parsed.dig "data", "id"
    parameters
  end
end
