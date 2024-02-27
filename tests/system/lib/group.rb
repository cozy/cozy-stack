class Group
  include Model

  attr_reader :name

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
    {
      name: @name
    }
  end
end
