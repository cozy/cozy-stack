class Instance
  attr_reader :stack, :name, :domain, :passphrase, :email, :client

  def self.create(opts = {})
    stack = Stack.get opts.delete(:port)
    inst = Instance.new stack, opts
    stack.start
    stack.create_instance inst
    inst
  end

  def initialize(stack, opts = {})
    @stack = stack
    @name = opts[:name] || Faker::Name.first_name
    @domain = opts[:domain] || "#{@name.downcase}.test.cozy.tools:#{stack.port}"
    @passphrase = opts[:passphrase] || "cozy"
    @email = opts[:email] || "#{@name.downcase}+test@cozy.tools"
    @client = RestClient::Resource.new "http://#{@domain}"
  end

  def url_for(obj = nil)
    case obj
    when Contact
      @stack.install_app self, "contacts"
      "http://contacts.#{@domain}/"
    else
      "http://#{@domain}/"
    end
  end

  def open(obj = nil, opts = {})
    browser = opts[:browser] || ENV['BROWSER'] || 'firefox'
    case browser
    when /firefox/
      `#{browser} -private-window #{url_for obj}`
    when /chrom/
      `#{browser} --incognito #{url_for obj}`
    else
      `#{browser} #{url_for obj}`
    end
  end

  def create_doc(doc)
    token = @stack.token_for self, [doc.doctype]
    opts = {
      content_type: :json,
      accept: :json,
      authorization: "Bearer #{token}"
    }
    body = JSON.generate doc.as_json
    @client["/data/#{doc.doctype}/"].post body, opts
  end
end
