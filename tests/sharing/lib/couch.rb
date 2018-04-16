class Couch
  def initialize(opts = {})
    @url = opts.delete(:url) || "http://localhost:5984"
    @client = RestClient::Resource.new @url, opts
  end

  def clean_test_dbs
    all_dbs.grep(/test[^a-zA-Z]/).each do |db|
      @client["/#{db.sub '/', '%2f'}"].delete
    end
  end

  def all_dbs
    JSON.parse @client["/_all_dbs"].get.body
  end
end
