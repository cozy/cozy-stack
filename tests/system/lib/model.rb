module Model
  module ClassMethods
    def create(inst, opts = {})
      obj = new opts
      obj.save inst
      obj
    end

    def find(inst, id)
      opts = {
        accept: :json,
        authorization: "Bearer #{inst.token_for doctype}"
      }
      res = inst.client["/data/#{doctype}/#{id}"].get opts
      from_json JSON.parse(res.body)
    end

    def all(inst)
      opts = {
        accept: :json,
        authorization: "Bearer #{inst.token_for doctype}"
      }
      res = inst.client["/data/#{doctype}/_all_docs?include_docs=true"].get opts
      JSON.parse(res.body)["rows"]
          .reject { |r| r["id"] =~ /^_design/ }
          .map { |r| from_json r["doc"] }
    end

    def changes(inst, since = nil)
      opts = {
        accept: :json,
        authorization: "Bearer #{inst.token_for doctype}"
      }
      url = "/data/#{doctype}/_changes?include_docs=true"
      url = "#{url}&since=#{since}" if since
      res = inst.client[url].get opts
      JSON.parse(res.body)
    end
  end

  def to_json(*_args)
    JSON.generate as_json
  end

  def as_reference
    {
      type: doctype,
      id: @couch_id
    }
  end

  def save(inst)
    opts = {
      content_type: :json,
      accept: :json,
      authorization: "Bearer #{inst.token_for doctype}"
    }
    res = if couch_id
            inst.client["/data/#{doctype}/#{couch_id}"].put to_json, opts
          else
            inst.client["/data/#{doctype}/"].post to_json, opts
          end
    j = JSON.parse(res.body)
    @couch_id = j["id"]
    @couch_rev = j["rev"]
  end

  def delete(inst)
    opts = {
      accept: "application/vnd.api+json",
      content_type: "application/vnd.api+json",
      authorization: "Bearer #{inst.token_for doctype}"
    }
    inst.client["/data/#{doctype}/#{@couch_id}?rev=#{@couch_rev}"].delete opts
  end

  def doctype
    self.class.doctype
  end

  def self.included(klass)
    klass.extend ClassMethods
    klass.send :attr_accessor, :couch_id, :couch_rev
  end


  module Files
    module ClassMethods
      def doctype
        "io.cozy.files"
      end

      def get_id_from_path(inst, path)
        find_by_path(inst, path).couch_id
      end

      def find_by_path(inst, path)
        load_from_url inst, "/files/metadata?Path=#{CGI.escape path}"
      end

      def find(inst, id)
        load_from_url inst, "/files/#{id}"
      end
    end

    def self.included(klass)
      klass.include Model
      klass.extend ClassMethods
    end

    def rename(inst, name)
      patch inst, name: name
      @name = name
    end

    def move_to(inst, dir_id)
      patch inst, dir_id: dir_id
      @dir_id = dir_id
    end

    def remove(inst)
      opts = {
        accept: "application/vnd.api+json",
        content_type: "application/vnd.api+json",
        authorization: "Bearer #{inst.token_for doctype}"
      }
      inst.client["/files/#{@couch_id}"].delete opts
    end

    def patch(inst, attrs)
      body = {
        data: {
          type: "io.cozy.files",
          id: @couch_id,
          attributes: attrs
        }
      }
      opts = {
        accept: "application/vnd.api+json",
        content_type: "application/vnd.api+json",
        authorization: "Bearer #{inst.token_for doctype}"
      }
      res = inst.client["/files/#{@couch_id}"].patch body.to_json, opts
      j = JSON.parse(res.body)["data"]
      @couch_rev = j["meta"]["rev"]
      @cozy_metadata = j["attributes"]["cozyMetadata"]
    rescue RestClient::InternalServerError => e
      puts "InternalServerError: #{e.http_body}"
      raise e
    end

    def restore(inst)
      restore_from_trash inst
    end
  end
end
