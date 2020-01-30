class Note
  attr_reader :file, :title, :dir_id, :schema

  include Model

  def self.doctype
    "io.cozy.notes.documents"
  end

  def self.default_schema
    {
      "nodes": [
        ["doc", { "content": "block+" }],
        ["paragraph", { "content": "inline*", "group": "block" }],
        ["blockquote", { "content": "block+", "group": "block" }],
        ["horizontal_rule", { "group": "block" }],
        ["heading", { "content": "inline*", "group": "block" }],
        ["text", { "group": "inline" }],
        ["hard_break", { "group": "inline", "inline": true }]
      ],
      "marks": [
        ["link", { "attrs": { "href": {}, "title": {} }, "inclusive": false }],
        ["em", {}],
        ["strong", {}],
        ["code", {}]
      ],
      "topNode": "doc"
    }
  end

  def self.open(inst, id)
    opts = {
      accept: 'application/vnd.api+json',
      authorization: "Bearer #{inst.token_for CozyFile.doctype}"
    }
    res = inst.client["/notes/#{id}/open"].get opts
    JSON.parse(res.body).dig "data", "attributes"
  end

  def initialize(opts = {})
    @title = opts[:title] || Faker::DrWho.quote
    @dir_id = opts[:dir_id] || Folder::ROOT_DIR
    @schema = opts[:schema] || Note.default_schema
  end

  def save(inst)
    opts = {
      content_type: 'application/vnd.api+json',
      authorization: "Bearer #{inst.token_for CozyFile.doctype}"
    }
    res = inst.client["/notes"].post to_jsonapi, opts
    @file = CozyFile.parse_jsonapi(res.body)
  end

  def as_json
    {
      title: @title,
      dir_id: @dir_id,
      schema: @schema
    }
  end

  def to_jsonapi
    JSON.generate data: { type: Note.doctype, attributes: as_json }
  end
end
