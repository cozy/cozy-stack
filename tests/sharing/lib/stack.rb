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
  end

  def start
    cmd = ["cozy-stack", "serve", "--log-level", "debug",
           "--mail-disable-tls", "--mail-port", "1025",
           "--port", @port, "--admin-port", @admin]
    Helpers.spawn cmd.join(" "), log: "stack-#{@port}.log"
    sleep 1
  end

  def create_instance(inst)
    cmd = ["cozy-stack", "instances", "add", inst.domain, "--dev",
           "--passphrase", inst.passphrase, "--public-name", inst.name,
           "--email", inst.email, "--settings", "context:test",
           "--admin-port", @admin]
    puts cmd.join(" ").green
    system cmd.join(" ")
  end
end
