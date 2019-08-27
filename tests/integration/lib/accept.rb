class Accept
  def initialize(sharing, sharer = nil)
    @sharing = sharing
    @owner = @sharing.members.first
    @sharer = sharer || @owner
  end

  def on(inst)
    @inst = inst
    state = extract_state_code
    location = do_discovery state
    sessid = connect_to_instance
    click_on_accept state, location, sessid
  end

  def extract_state_code
    doctype = "io-cozy-sharings"
    doc = Couch.new.get_doc @owner.domain, doctype, @sharing.couch_id
    idx = doc["members"].find_index { |m| %w(pending mail-not-sent).include? m["status"] }
    doc["credentials"][idx-1]["state"] if idx
  end

  def do_discovery(code)
    @sharer.client["/sharings/#{@sharing.couch_id}/discovery?state=#{code}"].get
    body = { state: code, url: @inst.url }
    res = @sharer.client["/sharings/#{@sharing.couch_id}/discovery"].post body, accept: :json
    JSON.parse(res.body)["redirect"]
  end

  def connect_to_instance
    client = RestClient::Resource.new @inst.url
    res = client["/auth/login"].get
    csrf_token = res.cookies["_csrf"]
    body = { csrf_token: csrf_token, passphrase: @inst.passphrase }
    params = { cookies: res.cookies, accept: :json }
    res2 = client["/auth/login"].post body, params
    res2.cookies["cozysessid"]
  end

  def click_on_accept(state, location, sessid)
    res = RestClient.get location, cookies: { cozysessid: sessid }
    csrf_token = res.cookies["_csrf"]
    client = RestClient::Resource.new @inst.url
    body = { csrf_token: csrf_token, state: state, sharing_id: @sharing.couch_id }
    params = { cookies: res.cookies, accept: :json }
    client["/auth/authorize/sharing"].post body, params
  end
end
