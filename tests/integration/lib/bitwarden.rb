class Bitwarden
  @number = 0

  def self.next_number
    @number += 1
  end

  def initialize(inst)
    @inst = inst
    @dir = File.join Helpers.current_dir, "bw_#{Bitwarden.next_number}"
  end

  def exec(cmd, session = true)
    cmd = "#{cmd} --session '#{@session}'" if session
    `BITWARDENCLI_APPDATA_DIR='#{@dir}' bw #{cmd}`.chomp
  end

  def login
    `mkdir -p #{@dir}`
    exec "config server http://#{@inst.domain}/bitwarden", false
    # If we were logged in from a previous run, we need to logout before trying
    # to log in, or we won't get a session key.
    logout
    domain = @inst.domain.split(':')[0]
    @session = exec "login --raw me@#{domain} #{@inst.passphrase}", false
  end

  def logout
    exec "logout", false
  end

  def sync
    exec "sync --force"
  end

  def json_exec(cmd)
    JSON.parse exec(cmd), symbolize_names: true
  end

  def get(object, id)
    json_exec "get #{object} #{id}"
  end

  def template(id)
    get :template, id
  end

  def list(object)
    json_exec "list #{object}"
  end

  def items
    list :items
  end

  def folders
    list :folders
  end

  def organizations
    list :organizations
  end

  def collections
    list :collections
  end

  def capture(cmd, data)
    env = { 'BITWARDENCLI_APPDATA_DIR' => @dir }
    out, err, status = Open3.capture3(env, "bw #{cmd}", stdin_data: data)
    unless status.success?
      puts "Stderr: #{err}"
      raise "Status code #{code} for bw #{cmd}"
    end
    out
  end

  def encode(data)
    capture "encode", data.to_json
  end

  def create(object, data)
    capture "create #{object} --session '#{@session}'", encode(data)
  end

  def create_folder(name)
    create :folder, name: name
  end
end
