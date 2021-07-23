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

  def force_sync
    exec "sync -f"
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

  class Organization
    attr_reader :id, :name, :key

    def self.doctype
      "com.bitwarden.organizations"
    end

    def self.create(inst, name)
      opts = {
        accept: "application/json",
        content_type: "application/json",
        authorization: "Bearer #{inst.token_for doctype}"
      }
      key = generate_key
      encrypted_key = encrypt_key(inst, key)
      encrypted_name = encrypt_name(key, name)
      data = {
        name: name,
        key: encrypted_key,
        collectionName: encrypted_name
      }
      body = JSON.generate data
      res = inst.client["/bitwarden/api/organizations"].post body, opts
      body = JSON.parse(res.body)
      Organization.new(id: body["Id"], name: body["Name"], key: key)
    end

    def self.generate_key
      Random.bytes 64
    end

    def self.encrypt_key(inst, key)
      private_key = get_private_key inst
      encoded_private_key = Base64.strict_encode64 private_key
      encoded_payload = Base64.strict_encode64 key
      result = `cozy-stack tools encrypt-with-rsa '#{encoded_private_key}' '#{encoded_payload}'`
      code = $?
      ap "Status code #{code} for encrypt-with-rsa" unless code.success?
      result.chomp
    end

    def self.get_private_key(inst)
      opts = {
        accept: "application/json",
        authorization: "Bearer #{inst.token_for 'com.bitwarden.profiles'}"
      }
      res = inst.client["/bitwarden/api/accounts/profile"].get opts
      body = JSON.parse(res.body)
      sym_key = decrypt_symmetric_key(inst, body["Key"])
      decrypt_private_key(sym_key, body["PrivateKey"])
    end

    def self.decrypt_symmetric_key(inst, encrypted)
      master_key = PBKDF2.new do |p|
        p.password = inst.passphrase
        p.salt = "me@" + inst.domain.split(':').first
        p.iterations = 100_000 # See pkg/crypto/pbkdf2.go
        p.hash_function = OpenSSL::Digest::SHA256
        p.key_length = 256 / 8
      end.bin_string

      iv, data = encrypted.sub("0.", "").split("|")
      iv = Base64.strict_decode64(iv)
      data = Base64.strict_decode64(data)

      cipher = OpenSSL::Cipher.new "AES-256-CBC"
      cipher.decrypt
      cipher.key = master_key
      cipher.iv = iv
      decrypted = cipher.update(data)
      decrypted << cipher.final
      decrypted
    end

    def self.decrypt_private_key(sym_key, encrypted)
      iv, data, mac = encrypted.sub("2.", "").split("|")
      iv = Base64.strict_decode64(iv)
      data = Base64.strict_decode64(data)

      cipher = OpenSSL::Cipher.new "AES-256-CBC"
      cipher.decrypt
      cipher.key = sym_key[0...32]
      cipher.iv = iv
      decrypted = cipher.update(data)
      decrypted << cipher.final

      computed_mac = OpenSSL::HMAC.digest("SHA256", sym_key[32...64], iv + data)
      encoded_mac = Base64.strict_encode64(computed_mac)
      raise "Invalid mac" if encoded_mac != mac

      decrypted
    end

    def self.encrypt_name(key, name)
      enc_key = key[0...32]
      mac_key = key[32...64]
      iv = Random.bytes 16
      cipher = OpenSSL::Cipher.new "AES-256-CBC"
      cipher.encrypt
      cipher.key = enc_key
      cipher.iv = iv
      data = cipher.update(name)
      data << cipher.final
      mac = OpenSSL::HMAC.digest("SHA256", mac_key, iv + data)
      "2.#{Base64.strict_encode64 iv}|#{Base64.strict_encode64 data}|#{Base64.strict_encode64 mac}"
    end

    def initialize(opts)
      @id = opts[:id]
      @name = opts[:name] || Faker::DrWho.character
      @key = opts[:key]
    end
  end
end
