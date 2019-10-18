class Mailhog
  def self.start
    `MailHog --version`
    spawn "MailHog"
  rescue Errno::ENOENT
    # Ignored: on our CI environment, MailHog is started as a docker service
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
