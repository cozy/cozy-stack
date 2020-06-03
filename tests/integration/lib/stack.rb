class Stack
  ALERT_ADDR = "alert@spam.cozycloud.cc".freeze

  class StackError < StandardError
    def message
      "Stack is not available"
    end
  end

  attr_reader :port

  @stacks = {}
  @next_port = ENV.fetch("COZY_BASE_PORT", 8080).to_i

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

  def konnectors_cmd
    File.expand_path "../../../scripts/konnector-node-run.sh", __dir__
  end

  def start
    vault = File.join Helpers.current_dir, "vault"
    FileUtils.mkdir_p vault
    fsurl = "file://#{Helpers.current_dir}/"
    if ENV["COZY_SWIFTTEST"]
      ap "SWIFT"
      fsurl = "'swift://127.0.0.1:6006/v1.0?UserName=swifttest&Password=swifttest&AuthURL=http://127.0.0.1:6006/v1.0'"
    end
    system("cozy-stack config gen-keys '#{vault}/key'") unless File.exist?("#{vault}/key.enc")
    cmd = ["cozy-stack", "serve", "--log-level", "debug",
           "--mailhog",
           "--port", @port, "--admin-port", @admin,
           "--fs-url", fsurl,
           "--vault-encryptor-key", "#{vault}/key.enc",
           "--vault-decryptor-key", "#{vault}/key.dec",
           "--mail-alert-address", ALERT_ADDR,
           "--konnectors-cmd", konnectors_cmd]
    Helpers.spawn cmd.join(" "), log: "stack-#{@port}.log"
    sleep 1
  end

  def create_instance(inst)
    cmd = ["cozy-stack", "instances", "add", inst.domain,
           "--passphrase", inst.passphrase, "--public-name", inst.name,
           "--email", inst.email, "--settings", "context:test",
           "--admin-port", @admin, "--locale", inst.locale]
    puts cmd.join(" ").green
    return if system(cmd.join(" "))
    # Try again if the cozy-stack serve was too slow to listen
    sleep 5
    Helpers.cat "stack-#{@port}.log"
    return if system(cmd.join(" "))
    raise StackError.new
  end

  def remove_instance(inst)
    cmd = ["cozy-stack", "instances", "rm", "--force", inst.domain,
           "--admin-port", @admin]
    puts cmd.join(" ").green
    system cmd.join(" ")
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

  def install_konnector(inst, slug, source_url = nil)
    cmd = ["cozy-stack", "konnectors", "install",
           slug, source_url,
           "--port", @port, "--admin-port", @admin,
           "--domain", inst.domain, ">", "/dev/null"].compact
    puts cmd.join(" ").green
    system cmd.join(" ")
  end

  def remove_konnector(inst, slug)
    cmd = ["cozy-stack", "konnectors", "uninstall", slug,
           "--port", @port, "--admin-port", @admin,
           "--domain", inst.domain, ">", "/dev/null"]
    puts cmd.join(" ").green
    system cmd.join(" ")
  end

  def run_konnector(inst, slug, account_id)
    cmd = ["cozy-stack", "konnectors", "run", slug,
           "--account-id", account_id,
           "--port", @port, "--admin-port", @admin,
           "--domain", inst.domain]
    puts cmd.join(" ").green
    out = `#{cmd.join(" ")}`.chomp
    Job.new JSON.parse(out)
  end

  def run_job(inst, type, args)
    cmd = ["cozy-stack", "jobs", "run", type,
           "--json", "'#{JSON.generate(args)}'",
           "--port", @port, "--admin-port", @admin,
           "--domain", inst.domain]
    puts cmd.join(" ").green
    out = `#{cmd.join(" ")}`.chomp
    Job.new JSON.parse(out)
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
           "http://localhost", "test-sharing", "github.com/cozy/cozy-stack/tests/integration"]
    puts cmd.join(" ").green
    `#{cmd.join(" ")}`.chomp
  end

  def fsck(inst)
    cmd = ["cozy-stack", "check", "fs", inst.domain,
           "--admin-port", @admin]
    puts cmd.join(" ").green
    `#{cmd.join(" ")}`.chomp.lines
  end

  def check_shared(inst)
    cmd = ["cozy-stack", "check", "shared", inst.domain,
           "--admin-port", @admin]
    puts cmd.join(" ").green
    `#{cmd.join(" ")}`.chomp.lines
  end

  def check(inst)
    [fsck(inst), check_shared(inst)].flatten
  end
end
