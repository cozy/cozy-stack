class Group
  include Model

  attr_accessor :name

  def self.doctype
    "io.cozy.contacts.groups"
  end

  def initialize(opts = {})
    @name = opts[:name] || Faker::Educator.subject
  end

  def self.from_json(j)
    group = Group.new(name: j["name"])
    group.couch_id = j["_id"]
    group.couch_rev = j["_rev"]
    group
  end

  def as_json
    if @couch_id && @couch_rev
      {
        _id: @couch_id,
        _rev: @couch_rev,
        name: @name
      }
    else
      {
        name: @name
      }
    end
  end
end
