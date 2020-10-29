class Export
  def doctype
    "io.cozy.exports"
  end

  def initialize(inst)
    @inst = inst
  end

  def run
    body = JSON.generate data: { attributes: {} }
    opts = {
      accept: "application/vnd.api+json",
      authorization: "Bearer #{@inst.token_for doctype}"
    }
    @inst.client["/move/exports"].post body, opts
  end

  def get_link(timeout = 20)
    timeout.times do
      sleep 1
      received = Email.received kind: "to", query: @inst.email
      return extract_link received.first if received.any?
    end
    raise "Export mail was not received after #{timeout} seconds"
  end

  def extract_link(mail)
    parts = mail.body.split "\r\n"
    parts.shift while parts.first !~ /^http:/
    parts.map { |p| p.chomp "=" }.join('')
  end
end
