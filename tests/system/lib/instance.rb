class Instance
  attr_reader :stack, :name, :domain, :email, :locale
  attr_accessor :passphrase

  def self.create(opts = {})
    stack = Stack.get opts.delete(:port)
    inst = Instance.new stack, opts
    stack.start
    stack.create_instance inst
    inst
  end

  def initialize(stack, opts = {})
    @stack = stack
    @name = opts[:name] || Faker::Internet.domain_word
    @domain = opts[:domain] || "#{@name.downcase}.test.localhost:#{stack.port}"
    @passphrase = opts[:passphrase] || "cozy" unless opts[:onboarded] == false
    @email = opts[:email] || "#{@name.downcase}+test@localhost"
    @locale = opts[:locale] || "fr"
  end

  def remove
    @stack.remove_instance self
  end

  def install_app(slug)
    @stack.install_app self, slug
  end

  def install_konnector(slug, source_url = nil)
    @stack.install_konnector self, slug, source_url
  end

  def remove_konnector(slug)
    @stack.remove_konnector self, slug
  end

  def run_konnector(slug, account_id)
    @stack.run_konnector self, slug, account_id
  end

  def run_job(type, args)
    @stack.run_job self, type, args
  end

  def setup_2fa
    @stack.setup_2fa self
  end

  def get_two_factor_code_from_mail(timeout = 30)
    timeout.times do
      sleep 1
      received = Email.received kind: "to", query: email
      received.select! { |e| e.subject =~ /One-time connection code/ }
      return extract_two_factor_code received.first if received.any?
    end
    raise "Mail with two-factor code was not received after #{timeout} seconds"
  end

  def extract_two_factor_code(mail)
    scanned = mail.body.scan(/: (\d{6})/)
    raise "No code in #{mail.subject} - #{mail.body}" if scanned.empty?
    scanned.last.last
  end

  def client
    @client ||= RestClient::Resource.new url
  end

  def url(obj = nil)
    case obj
    when Contact
      @stack.install_app self, "contacts"
      "http://contacts.#{@domain}/"
    when Folder
      @stack.install_app self, "drive"
      "http://drive.#{@domain}/"
    else
      "http://#{@domain}"
    end
  end

  def open(obj = nil, opts = {})
    browser = opts[:browser] || ENV['BROWSER'] || 'firefox'
    case browser
    when /firefox/
      `#{browser} -private-window #{url obj}`
    when /chrom/
      `#{browser} --incognito #{url obj}`
    else
      `#{browser} #{url obj}`
    end
  end

  def token_for(doctype)
    @stack.token_for self, [doctype]
  end

  def register(sharing)
    doctypes = sharing.rules.map(&:doctype).uniq
    token = @stack.token_for self, doctypes
    opts = {
      accept: "application/vnd.api+json",
      content_type: "application/vnd.api+json",
      authorization: "Bearer #{token}"
    }
    body = JSON.generate sharing.as_json_api
    res = @client["/sharings/"].post body, opts
    sharing.couch_id = JSON.parse(res.body)["data"]["id"]
  end

  def accept(sharing, sharer = nil)
    @stack.install_app self, "home"
    Accept.new(sharing, sharer).on self
  end

  def version
    res = @client["/version/"].get accept: "application/json"
    JSON.parse(res.body)
  end

  def fsck
    @stack.fsck self
  end

  def check
    @stack.check self
  end

  def open_session
    client = RestClient::Resource.new url
    res = client["/auth/login"].get
    csrf_token = res.cookies["_csrf"]
    body = { csrf_token: csrf_token, passphrase: hashed_passphrase }
    params = { cookies: res.cookies, accept: :json }
    res2 = client["/auth/login"].post body, params
    res2.cookies["cozysessid"]
  end

  # See https://github.com/jcs/rubywarden/blob/master/API.md#example
  def hashed_passphrase
    key = PBKDF2.new do |p|
      p.password = passphrase
      p.salt = "me@" + domain.split(':').first
      p.iterations = 100_000 # See pkg/crypto/pbkdf2.go
      p.hash_function = OpenSSL::Digest::SHA256
      p.key_length = 256 / 8
    end.bin_string
    hashed = PBKDF2.new do |p|
      p.password = key
      p.salt = passphrase
      p.iterations = 1
      p.hash_function = OpenSSL::Digest::SHA256
      p.key_length = 256 / 8
    end.bin_string
    Base64.strict_encode64 hashed
  end

  def register_token
    doc = Couch.new.instances.detect { |i| i["domain"] == @domain }
    token = Base64.decode64 doc["register_token"]
    Digest.hexencode token
  end
end
