class Bitwarden
  def initialize(inst)
    @inst = inst
  end

  def login
    `bw config server http://#{@inst.domain}/bitwarden`
    # If we were logged in from a previous run, we need to logout before trying
    # to log in, or we won't get a session key.
    logout
    domain = @inst.domain.split(':')[0]
    @session = `bw login --raw me@#{domain} #{@inst.passphrase}`.chomp
  end

  def logout
    `bw logout`.chomp
  end

  def sync
    `bw sync --force --session '#{@session}'`.chomp
  end

  def json_exec(cmd)
    out = `#{cmd} --session '#{@session}'`.chomp
    JSON.parse out, symbolize_names: true
  end

  def get(object, id)
    json_exec "bw get #{object} #{id}"
  end

  def template(id)
    get :template, id
  end

  def list(object)
    json_exec "bw list #{object}"
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

  def encode(data)
    result = nil
    Open3.popen3("bw encode") do |stdin, stdout, _, wait|
      stdin << data.to_json
      stdin.close
      result = stdout.read.chomp
      code = wait.value
      raise "Status code #{code} for bw encode" unless code == 0
    end
    result
  end

  def create(object, data)
    result = nil
    Open3.popen3("bw create #{object} --session '#{@session}'") do |stdin, stdout, _, wait|
      stdin << encode(data)
      stdin.close
      result = stdout.read.chomp
      code = wait.value
      raise "Status code #{code} for bw encode" unless code == 0
    end
    result
  end

  def create_folder(name)
    create :folder, name: name
  end
end
