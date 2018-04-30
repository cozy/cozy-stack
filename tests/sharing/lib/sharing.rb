class Sharing
  attr_accessor :couch_id, :couch_rev, :rules
  attr_reader :description, :app_slug, :rules, :members

  def self.get_sharing_info(inst, sharing_id, doctype)
    opts = {
      accept: "application/vnd.api+json",
      content_type: "application/vnd.api+json",
      authorization: "Bearer #{inst.token_for doctype}"
    }
    res = inst.client["/sharings/#{sharing_id}"].get opts
    JSON.parse(res.body)["data"]
  end

  def self.get_shared_docs(inst, sharing_id, doctype)
    j = get_sharing_info inst, sharing_id, doctype
    j.dig "relationships", "shared_docs", "data"
  end

  def initialize(opts = {})
    @description = opts[:description] || Faker::HitchhikersGuideToTheGalaxy.marvin_quote
    @app_slug = opts[:app_slug] || ""
    @rules = []
    @members = [] # Owner's instance + recipients contacts
  end

  def self.doctype
    "io.cozy.sharings"
  end

  def as_json_api
    recipients = @members.drop 1
    {
      data: {
        doctype: self.class.doctype,
        attributes: {
          description: @description,
          app_slug: @app_slug,
          rules: @rules.map(&:as_json)
        },
        relationships: {
          recipients: {
            data: recipients.map(&:as_reference)
          }
        }
      }
    }
  end

  def owner
    @members.first
  end
end
