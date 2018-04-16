class Sharing
  attr_reader :description, :app_slug, :rules

  def initialize(opts = {})
    @description = opts[:description] || Faker::HitchhikersGuideToTheGalaxy.marvin_quote
    @app_slug = opts[:app_slug] || ""
    @rules = []
  end

  def doctype
    "io.cozy.sharings"
  end

  def as_json_api
    {
      data: {
        doctype: doctype,
        attributes: {
          description: @description,
          app_slug: @app_slug,
          rules: @rules.map(&:as_json)
        }
      }
    }
  end
end
