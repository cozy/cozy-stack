class Notification
  attr_reader :title

  def initialize(title)
    @title = title
  end

  def self.doctype
    "io.cozy.notifications"
  end

  def self.create(inst, at=nil)
    msg = Faker::Friends.quote
    title = Faker::DrWho.quote
    title = title.gsub(/\W+/, ' ')
    attrs = {
      category: "balance-lower",
      title: title,
      message: msg,
      content: msg,
      content_html: msg
    }
    attrs[:at] = at unless at.nil?
    body = JSON.generate data: { attributes: attrs }
    opts = {
      accept: "application/vnd.api+json",
      authorization: "Bearer #{inst.token_for doctype}"
    }
    inst.client["/notifications"].post body, opts
    Notification.new(title: title)
  end

  def self.received(params)
    url = "http://localhost:8025/api/v2" # MailHog
    client = RestClient::Resource.new url
    res = client["/search"].get params: params
    JSON.parse(res.body)["items"].map do |item|
      title = item.dig "Content", "Headers", "Subject", 0
      Notification.new(title: title)
    end
  end
end
