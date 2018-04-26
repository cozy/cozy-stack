class Stack
  attr_reader :port

  @stacks = {}
  @next_port = 8080

  def self.get(port = nil)
    port ||= (@next_port += 1)
    @stacks[port] ||= Stack.new(port)
  end

  def initialize(port)
    @port = port
    @admin = port - 2020
    @oauth_client_id = nil
    @apps = {}
    @tokens = {}
  end

  def start
    cmd = ["cozy-stack", "serve", "--log-level", "debug",
           "--mail-disable-tls", "--mail-port", "1025",
           "--port", @port, "--admin-port", @admin,
           "--fs-url", "file://#{Helpers.current_dir}/"]
    Helpers.spawn cmd.join(" "), log: "stack-#{@port}.log"
    sleep 1
  end

  def create_instance(inst)
    cmd = ["cozy-stack", "instances", "add", inst.domain, "--dev",
           "--passphrase", inst.passphrase, "--public-name", inst.name,
           "--email", inst.email, "--settings", "context:test",
           "--admin-port", @admin.to_s, "--locale", "fr"]
    puts cmd.join(" ").green
    system(*cmd)
  end

  def install_app(inst, app)
    key = inst.domain + "/" + app
    return if @apps[key]
    cmd = ["cozy-stack", "apps", "install", app,
           "--port", @port, "--admin-port", @admin,
           "--domain", inst.domain, ">", "/dev/null"]
    puts cmd.join(" ").green
    @apps[key] = system cmd.join(" ")
  end

  def token_for(inst, doctypes)
    key = inst.domain + "/" + doctypes.join(" ")
    @tokens[key] ||= generate_token_for(inst, doctypes)
  end

  def generate_token_for(inst, doctypes)
    @oauth_client_id ||= generate_client_id(inst)
    cmd = ["cozy-stack", "instances", "token-oauth", inst.domain,
           "--admin-port", @admin,
           @oauth_client_id, "'#{doctypes.join(' ')}'"]
    puts cmd.join(" ").green
    `#{cmd.join(" ")}`.chomp
  end

  def generate_client_id(inst)
    cmd = ["cozy-stack", "instances", "client-oauth", inst.domain,
           "--admin-port", @admin,
           "http://localhost", "test-sharing", "github.com/cozy/cozy-stack/tests/sharing"]
    puts cmd.join(" ").green
    `#{cmd.join(" ")}`.chomp
  end
end
