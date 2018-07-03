class Instance
  attr_reader :stack, :name, :domain, :passphrase, :email

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
    @domain = opts[:domain] || "#{@name.downcase}.test.cozy.tools:#{stack.port}"
    @passphrase = opts[:passphrase] || "cozy"
    @email = opts[:email] || "#{@name.downcase}+test@cozy.tools"
  end

  def install_app(slug)
    @stack.install_app self, slug
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
    @stack.install_app self, "drive"
    Accept.new(sharing, sharer).on self
  end
end
