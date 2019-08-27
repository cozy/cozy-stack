class Couch
  def initialize(opts = {})
    @url = opts.delete(:url) || "http://localhost:5984"
    @client = RestClient::Resource.new @url, opts
  end

  def clean_test
    instances.each do |inst|
      next unless inst["domain"] =~ /test[^a-zA-Z]/
      all_dbs.grep(/#{prefix inst['domain']}/).each do |db|
        @client["/#{db.sub '/', '%2f'}"].delete
      end
      params = { params: { rev: inst["_rev"] } }
      @client["/global%2Finstances/#{inst['_id']}"].delete params
    end
  end

  def all_dbs
    JSON.parse @client["/_all_dbs"].get.body
  end

  def instances
    params = { params: { include_docs: true } }
    res = JSON.parse @client["/global%2Finstances/_all_docs"].get(params).body
    res["rows"].map { |row| row["doc"] }
  rescue RestClient::NotFound
    []
  end

  def prefix(db)
    "cozy" + Digest::SHA256.hexdigest(db)[0...32]
  end

  def get_doc(domain, doctype, id)
    doctype = doctype.gsub(/\W/, '-')
    JSON.parse @client["/#{prefix domain}%2F#{doctype}/#{id}"].get.body
  end

  def create_named_doc(domain, doctype, id, doc)
    opts = {
      content_type: "application/json"
    }
    doctype = doctype.gsub(/\W/, '-')
    @client["/#{prefix domain}%2F#{doctype}/#{id}"].put(doc.to_json, opts)
  end

  def update_doc(domain, doctype, doc)
    opts = {
      content_type: "application/json"
    }
    id = doc["_id"]
    doctype = doctype.gsub(/\W/, '-')
    @client["/#{prefix domain}%2F#{doctype}/#{id}"].put(doc.to_json, opts)
  end
end
