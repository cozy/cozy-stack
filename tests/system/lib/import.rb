class Import
  def doctype
    "io.cozy.imports"
  end

  def initialize(inst, link)
    @inst = inst
    @link = link
  end

  def precheck
    body = JSON.generate data: { attributes: { url: @link } }
    opts = {
      accept: "application/vnd.api+json",
      authorization: "Bearer #{@inst.token_for doctype}"
    }
    @inst.client["/move/imports/precheck"].post body, opts
    # It raises an exception if the status code is 4xx
  end

  def run
    body = JSON.generate data: { attributes: { url: @link } }
    opts = {
      accept: "application/vnd.api+json",
      authorization: "Bearer #{@inst.token_for doctype}"
    }
    @inst.client["/move/imports"].post body, opts
  end

  def wait_done(timeout = 120)
    timeout.times do
      sleep 1
      received = Email.received kind: "to", query: @inst.email
      return if received.any?
    end
    raise "Import mail was not received after #{timeout} seconds"
  end
end
