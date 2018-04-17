class Rule
  attr_reader :title, :doctype, :selector, :values, :add, :update, :remove

  def self.sync(obj)
    create_from_obj obj, "sync"
  end

  def self.push(obj)
    create_from_obj obj, "push"
  end

  def self.create_from_obj(obj, what)
    case obj
    when Array
      values = obj.map(&:couch_id)
      doctype = obj.first.doctype
      title = obj.first.name rescue nil
    else
      values = [obj.couch_id]
      doctype = obj.doctype
      title = obj.name rescue nil
    end
    Rule.new doctype: doctype,
             title: title,
             values: values,
             add: what,
             update: what,
             remove: what
  end

  def initialize(opts = {})
    @title = opts[:title] || Faker::Hobbit.thorins_company
    @doctype = opts[:doctype]
    @selector = opts[:selector]
    @values = opts[:values]
    @add = opts[:add]
    @update = opts[:update]
    @remove = opts[:remove]
  end

  def as_json
    {
      title: @title,
      doctype: @doctype,
      selector: @selector,
      values: @values,
      add: @add,
      update: @update,
      remove: @remove
    }
  end
end
