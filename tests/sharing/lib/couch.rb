class Couch
  def initialize(opts = {})
    @url = opts.delete(:url) || "http://localhost:5984"
    @client = RestClient::Resource.new @url, opts
  end

  def clean_test
    all_dbs.grep(/test[^a-zA-Z]/).each do |db|
      @client["/#{db.sub '/', '%2f'}"].delete
    end
    instances.each do |inst|
      if inst["domain"] =~ /test[^a-zA-Z]/
        params = { params: { rev: inst["_rev"] } }
        @client["/global%2Finstances/#{inst["_id"]}"].delete params
      end
    end
  end

  def all_dbs
    JSON.parse @client["/_all_dbs"].get.body
  end

  def instances
    params = { params: { include_docs: true } }
    res = JSON.parse @client["/global%2Finstances/_all_docs"].get(params).body
    res["rows"].map {|row| row["doc"] }
  end
end
