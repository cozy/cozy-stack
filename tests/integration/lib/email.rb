class Email
  MAILHOG = 'http://localhost:8025/api'
  attr_reader :subject, :from, :to, :body

  def initialize(opts = {})
    @subject = opts[:subject]
    @from = opts[:from]
    @to = opts[:to]
    @body = opts[:body]
  end

  def self.client
    @client ||= RestClient::Resource.new MAILHOG
  end

  def self.clear_inbox
    client["/v1/messages"].delete
  end

  def self.received(params)
    client = RestClient::Resource.new MAILHOG
    res = client["/v2/search"].get params: params
    JSON.parse(res.body)["items"].map do |item|
      subject = item.dig "Content", "Headers", "Subject", 0
      from = item.dig "Content", "Headers", "From", 0
      to = item.dig "Content", "Headers", "From", 0
      body = item.dig "MIME", "Parts", 0, "Body"
      Email.new(subject: subject, from: from, to: to, body: body)
    end
  end
end
