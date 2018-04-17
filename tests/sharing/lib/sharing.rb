class Sharing
  attr_accessor :couch_id
  attr_reader :description, :app_slug, :rules, :members

  def initialize(opts = {})
    @description = opts[:description] || Faker::HitchhikersGuideToTheGalaxy.marvin_quote
    @app_slug = opts[:app_slug] || ""
    @rules = []
    @members = [] # Owner's instance + recipients contacts
  end

  def doctype
    "io.cozy.sharings"
  end

  def as_json_api
    recipients = @members.drop 1
    {
      data: {
        doctype: doctype,
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
