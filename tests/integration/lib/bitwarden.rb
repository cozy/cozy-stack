class Bitwarden
  module Types
    LOGIN = 1
    SECURENOTE = 2
    CARD = 3
    IDENTITY = 4
  end

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
    result = `BITWARDENCLI_APPDATA_DIR='#{@dir}' bw #{cmd}`.chomp
    code = $?
    ap "Status code #{code} for bw #{cmd}" unless code.success?
    result
  end

  def login
    `mkdir -p #{@dir}`
    exec "config server http://#{@inst.domain}/bitwarden", false
    domain = @inst.domain.split(':')[0]
    @session = exec "login --raw me@#{domain} #{@inst.passphrase}", false
  end

  def logout
    exec "logout", false
  end

  def sync
    exec "sync"
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

  def capture(cmd, data, session = true)
    cmd = "#{cmd} --session '#{@session}'" if session
    env = { 'BITWARDENCLI_APPDATA_DIR' => @dir }
    out, err, status = Open3.capture3(env, "bw #{cmd}", stdin_data: data)
    unless status.success?
      ap "Status code #{status} for bw #{cmd}"
      ap "Stderr: #{err}"
    end
    out
  end

  def encode(data)
    capture "encode", data.to_json, false
  end

  def create(object, data)
    capture "create #{object}", encode(data)
  end

  def create_folder(name)
    create :folder, name: name
  end

  def create_item(data)
    create :item, data
  end

  def edit(object, id, data)
    capture "edit #{object} #{id}", encode(data)
  end

  def edit_folder(id, name)
    edit :folder, id, name: name
  end

  def edit_item(id, data)
    edit :item, id, data
  end

  def delete(object, id)
    exec "delete #{object} #{id} --permanent"
  end

  def delete_folder(id)
    delete :folder, id
  end

  def delete_item(id)
    delete :item, id
  end

  def share(item_id, org_id, coll_id)
    capture "share #{item_id} #{org_id}", encode([coll_id])
  end
end
