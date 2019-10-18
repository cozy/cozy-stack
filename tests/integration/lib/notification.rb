class Notification
  attr_reader :title

  def initialize(opts = {})
    @title = opts[:title]
  end

  def self.doctype
    "io.cozy.notifications"
  end

  def self.create(inst, at = nil)
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
end
