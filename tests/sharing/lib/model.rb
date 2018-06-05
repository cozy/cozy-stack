module Model
  module ClassMethods
    def create(inst, opts = {})
      obj = new opts
      obj.save inst
      obj
    end
  end

  def to_json
    JSON.generate as_json
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
        load_from_url inst, "/files/metadata?Path=#{path}"
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
    end

    def restore(inst)
      restore_from_trash inst
    end
  end
end
