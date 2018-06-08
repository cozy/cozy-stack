class Rule
  attr_reader :title, :doctype, :selector, :values, :add, :update, :remove, :object

  def self.sync(obj)
    create_from_obj obj, "sync"
  end

  def self.push(obj)
    create_from_obj obj, "push"
  end

  def self.none(obj)
    create_from_obj obj, "none"
  end

  def self.create_from_album(obj, what)
    selector = "referenced_by"
    values = ["#{obj.doctype}/#{obj.couch_id}"]
    title = obj.name rescue nil
    doctype = "io.cozy.files"
    r1 = Rule.new doctype: doctype,
                  title: title,
                  values: values,
                  selector: selector,
                  add: what,
                  update: what,
                  remove: what,
                  object: obj
    values = [obj.couch_id]
    title = obj.name rescue nil
    doctype = obj.doctype
    r2 = Rule.new doctype: doctype,
                  title: title,
                  values: values,
                  add: what,
                  update: what,
                  remove: what,
                  object: obj
    [r1, r2]
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
             remove: what,
             object: obj
  end

  def initialize(opts = {})
    @title = opts[:title] || Faker::Hobbit.thorins_company
    @doctype = opts[:doctype]
    @selector = opts[:selector]
    @values = opts[:values]
    @add = opts[:add]
    @update = opts[:update]
    @remove = opts[:remove]
    @object = opts[:object]
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
