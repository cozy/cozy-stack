class Move
  def initialize(source, target)
    @source = source
    @target = target
  end

  def get_initialize_token
    opts = {
      max_redirects: 0,
      cookies: { cozysessid: @source.open_session }
    }
    @source.client["/move/initialize"].post(nil, opts) do |response|
      params = extract_query_string response.headers[:location]
      @source_client_id = params["client_id"]
      @source_client_secret = params["client_secret"]
      @source_token = access_token @source, params
    end
  end

  def get_source_token
    opts = {
      max_redirects: 0,
      cookies: { cozysessid: @source.open_session }
    }
    state = "123456789"
    qs = "state=#{state}&redirect_uri=#{redirect_uri}"
    @source.client["/move/authorize?#{qs}"].get(opts) do |response|
      params = extract_query_string response.headers[:location]
      @code = params["code"]
    end
  end

  def get_target_token
    client = create_oauth_client @target
    @target_client_id = client["client_id"]
    @target_client_secret = client["client_secret"]

    state = "123456789"
    opts = { accept: "application/json", cookies: { cozysessid: @source.open_session } }
    qs = "state=#{state}&client_id=#{@target_client_id}&redirect_uri=#{redirect_uri}"
    res = @target.client["/auth/authorize/move?#{qs}"].get opts
    csrf_token = res.cookies["_csrf"]

    body = {
      state: state,
      client_id: @target_client_id,
      csrf_token: csrf_token,
      redirect_uri: redirect_uri,
      passphrase: @target.hashed_passphrase
    }
    opts[:cookies] = res.cookies
    res = @target.client["/auth/authorize/move"].post(body, opts)
    redirect = JSON.parse(res.body)["redirect"]
    params = extract_query_string redirect
    @target_token = access_token @target, params.merge(client)
  end

  def create_oauth_client(inst)
    params = {
      client_name: "Cozy-Move",
      client_kind: "web",
      client_uri: "http://localhost:4000/",
      redirect_uris: [redirect_uri],
      software_id: "github.com/cozy/cozy-move",
      software_version: "0.0.1"
    }
    body = JSON.generate(params)
    opts = { accept: "application/json", "Content-Type": "application/json" }
    res = inst.client["/auth/register"].post body, opts
    JSON.parse(res.body)
  end

  def redirect_uri
    "http://localhost:4000/callback/target"
  end

  def access_token(inst, params)
    body = {
      grant_type: "authorization_code",
      code: params["code"],
      client_id: params["client_id"],
      client_secret: params["client_secret"]
    }
    opts = { accept: :json }
    res = inst.client["/auth/access_token"].post body, opts
    JSON.parse(res.body)["access_token"]
  end

  def extract_query_string(location)
    query = URI.parse(location).query
    CGI.parse(query).inject({}) do |h, (k, v)|
      h[k] = v.first
      h
    end
  end

  def run
    body = {
      target_url: @target.url,
      target_token: @target_token,
      target_client_id: @target_client_id,
      target_client_secret: @target_client_secret
    }
    if @code
      body[:code] = @code
    else
      body[:token] = @source_token
      body[:client_id] = @source_client_id
      body[:client_secret] = @source_client_secret
    end
    opts = { accept: "*/*" }
    @source.client["/move/request"].post body, opts
  end

  def confirm(timeout = 60)
    timeout.times do
      sleep 1
      received = Email.received kind: "to", query: @source.email
      return confirm_mail received.first if received.any?
    end
    raise "Confirmation mail for moving was not received after #{timeout} seconds"
  end

  def confirm_mail(mail)
    scanned = mail.body.scan(/secret=3D(\w+)/)
    raise "No secret in #{mail.body}" unless scanned
    secret = scanned.last.last
    opts = { cookies: { cozysessid: @source.open_session } }
    @source.client["/move/go?secret=#{secret}"].get opts
  end

  def wait_done(timeout = 60)
    timeout.times do
      sleep 1
      received = Email.received kind: "to", query: @target.email
      return if received.any?
    end
    raise "Moved mail was not received after #{timeout} seconds"
  end
end
