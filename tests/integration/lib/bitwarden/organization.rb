class Bitwarden
  class Organization
    attr_reader :id, :name, :key

    def self.doctype
      "com.bitwarden.organizations"
    end

    def initialize(opts)
      @id = opts[:id]
      @name = opts[:name] || Faker::DrWho.character
      @key = opts[:key]
    end

    def self.create(inst, name)
      opts = {
        accept: "application/json",
        content_type: "application/json",
        authorization: "Bearer #{inst.token_for doctype}"
      }
      org_key = generate_key
      user_key = get_private_key(inst)
      encrypted_key = encrypt_key(user_key, org_key)
      encrypted_name = encrypt_name(org_key, name)
      data = {
        name: name,
        key: encrypted_key,
        collectionName: encrypted_name
      }
      body = JSON.generate data
      res = inst.client["/bitwarden/api/organizations"].post body, opts
      body = JSON.parse(res.body)
      Organization.new(id: body["Id"], name: body["Name"], key: org_key)
    end

    def self.generate_key
      Random.bytes 64
    end

    def self.encrypt_key(user_key, org_key)
      encoded_payload = Base64.strict_encode64 org_key
      result = `cozy-stack tools encrypt-with-rsa '#{user_key}' '#{encoded_payload}'`
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
      decrypted = decrypt_private_key(sym_key, body["PrivateKey"])
      Base64.strict_encode64 decrypted
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
  end
end
