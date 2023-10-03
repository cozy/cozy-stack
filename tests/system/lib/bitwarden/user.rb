class Bitwarden
  class User
    attr_reader :id, :name, :email, :type, :status

    def initialize(opts)
      @id = opts[:id]
      @name = opts[:name]
      @email = opts[:email]
      @type = opts[:type]
      @status = opts[:status]
    end

    def self.list(inst, org_id)
      token = inst.token_for Bitwarden::Organization.doctype
      opts = {
        accept: "application/json",
        authorization: "Bearer #{token}"
      }
      path = "/bitwarden/api/organizations/#{org_id}/users"
      res = inst.client[path].get opts
      body = JSON.parse(res.body)
      body["Data"].map do |data|
        Bitwarden::User.new(
          id: data["UserId"],
          type: data["Type"],
          status: data["Status"],
          name: data["Name"],
          email: data["Email"]
        )
      end
    end

    def fetch_public_key(inst)
      token = inst.token_for Bitwarden::Organization.doctype
      opts = {
        accept: "application/json",
        authorization: "Bearer #{token}"
      }
      path = "/bitwarden/api/users/#{id}/public-key"
      res = inst.client[path].get opts
      body = JSON.parse(res.body)
      body["PublicKey"]
    end

    def confirm(inst, org_id, encrypted_key)
      token = inst.token_for Bitwarden::Organization.doctype
      opts = {
        accept: "application/json",
        content_type: "application/json",
        authorization: "Bearer #{token}"
      }
      data = { key: encrypted_key }
      body = JSON.generate data
      path = "/bitwarden/api/organizations/#{org_id}/users/#{id}/confirm"
      inst.client[path].post body, opts
    end
  end
end
