class Bitwarden
  module Cipher
    def self.doctype
      "com.bitwarden.ciphers"
    end

    # Bitwarden CLI doesn't let us remove a cipher from an organization, or
    # move a cipher from one organization to another. But, we can use the
    # update method manually.
    def self.update(inst, item_id, data)
      opts = {
        accept: "application/json",
        content_type: "application/json",
        authorization: "Bearer #{inst.token_for doctype}"
      }
      key = get_symmetric_key(inst)
      data[:name] = Bitwarden::Organization.encrypt_name(key, data[:name])
      data[:notes] = Bitwarden::Organization.encrypt_name(key, data[:notes] || "")
      data[:card].each do |field, value|
        data[:card][field] = Bitwarden::Organization.encrypt_name(key, value)
      end if data[:card]
      data[:organizationId] = nil
      data[:collectionsIds] = nil
      body = JSON.generate data
      res = inst.client["/bitwarden/api/ciphers/#{item_id}"].put body, opts
      JSON.parse(res.body)
    end

    def self.get_symmetric_key(inst)
      opts = {
        accept: "application/json",
        authorization: "Bearer #{inst.token_for 'com.bitwarden.profiles'}"
      }
      res = inst.client["/bitwarden/api/accounts/profile"].get opts
      body = JSON.parse(res.body)
      Bitwarden::Organization.decrypt_symmetric_key(inst, body["Key"])
    end
  end
end
