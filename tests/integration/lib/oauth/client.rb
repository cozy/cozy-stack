module OAuth
  class Client
    attr_reader :client_id, :client_secret, :registration_token

    def self.create(inst)
      body = {
        redirect_uris: ["cozy://"],
        client_name: "test_#{Faker::Internet.slug}",
        software_id: "github.com/cozy/cozy-stack/tests/integration"
      }
      opts = { content_type: :json, accept: :json }
      res = inst.client["/auth/register"].post body.to_json, opts
      body = JSON.parse(res.body)
      params = {
        client_id: body["client_id"],
        client_secret: body["client_secret"],
        registration_token: body["registration_access_token"]
      }
      new params
    end

    def initialize(opts)
      @client_id = opts[:client_id]
      @client_secret = opts[:client_secret]
      @registration_token = opts[:registration_token]
    end

    def register_passphrase(inst, pass)
      inst.passphrase = pass
      body = {
        client_id: @client_id,
        client_secret: @client_secret,
        register_token: inst.register_token,
        passphrase: inst.hashed_passphrase,
        iterations: 100_000,
        hint: "a hint to help me remember my passphrase",
        key: "dont_care_key",
        public_key: "dont_care_public_key",
        private_key: "dont_care_private_key"
      }
      opts = { content_type: :json, accept: :json }
      res = inst.client["/settings/passphrase/flagship"].post body.to_json, opts
      JSON.parse(res.body)["session_code"]
    end

    def open_authorize_page(inst, session_code)
      @authorize_params = [
        "session_code=#{session_code}",
        "client_id=#{@client_id}",
        "redirect_uri=cozy://",
        "response_type=code",
        "scope=*",
        "state=state",
        "code_challenge=LdAL134CIs7YgmZUganB2fkHMJ0W4F7QB6HqY5KEd6k", # "challenge"
        "code_challenge_method=S256"
      ].join('&')
      inst.client["/auth/authorize?#{@authorize_params}"].get
    end

    def receive_flagship_code(inst, timeout = 30)
      timeout.times do
        sleep 1
        received = Email.received kind: "to", query: inst.email
        received.reject! { |e| e.subject =~ /(New.connection|Nouvelle.connexion)/ }
        return extract_code received.first if received.any?
      end
      raise "Confirmation mail for moving was not received after #{timeout} seconds"
    end

    def extract_code(mail)
      scanned = mail.body.scan(/^(\d{6})$/)
      raise "No secret in #{mail.subject} - #{mail.body}" if scanned.empty?
      scanned.last.last
    end

    def validate_flagship(inst, authorize_page, code)
      opts = {
        max_redirects: 0,
        cookies: authorize_page.cookies
      }
      extracted = authorize_page.body.scan(/name="confirm-token" value="([^"]+)"/)
      body = { token: extracted.first.first, code: code }
      inst.client["/auth/clients/#{@client_id}/flagship"].post body, opts
      inst.client["/auth/authorize?#{@authorize_params}"].get(opts) do |response|
        params = extract_query_string response.headers[:location]
        params["code"]
      end
    end

    def access_token(inst, access_code)
      body = {
        grant_type: "authorization_code",
        code: access_code,
        client_id: @client_id,
        client_secret: @client_secret,
        code_verifier: "challenge"
      }
      opts = { accept: :json }
      res = inst.client["/auth/access_token"].post body, opts
      JSON.parse(res.body)
    end

    def extract_query_string(location)
      query = URI.parse(location).query
      CGI.parse(query).inject({}) do |h, (k, v)|
        h[k] = v.first
        h
      end
    end

    def list_permissions(inst, tokens)
      access_token = tokens["access_token"]
      opts = {
        accept: :json,
        authorization: "Bearer #{access_token}"
      }
      res = inst.client["/permissions/self"].get opts
      JSON.parse(res.body)
    end

    def create_session_code(inst)
      body = { passphrase: inst.hashed_passphrase }
      opts = { accept: :json, content_type: :json }
      begin
        res = inst.client["/auth/session_code"].post body.to_json, opts
      rescue RestClient::Exception => e
        token = JSON.parse(e.response.body)["two_factor_token"]
        code = inst.get_two_factor_code_from_mail
        body = {
          passphrase: inst.hashed_passphrase,
          two_factor_token: token,
          two_factor_passcode: code
        }
        res = inst.client["/auth/session_code"].post body.to_json, opts
      end
      body = JSON.parse(res.body)
      body["session_code"]
    end

    def destroy(inst)
      opts = { authorization: "Bearer #{@registration_token}" }
      inst.client["/auth/register/#{@client_id}"].delete opts
    end
  end
end
