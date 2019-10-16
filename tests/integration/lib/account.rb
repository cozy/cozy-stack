class Account
  include Model

  attr_reader :name, :log

  def self.doctype
    "io.cozy.accounts"
  end

  def initialize(opts = {})
    @couch_id = opts[:id]
    @name = (opts[:name] || Faker::DrWho.character).gsub(/[^A-Za-z]/, '_')
    @log = opts[:log] || "#{Helpers.current_dir}/account_#{@name}.log"
    @aggregator = opts[:aggregator]
    @type = opts[:type]
  end

  def as_json
    json = {
      name: @name,
      log: @log,
      account_type: @type
    }.compact
    if @aggregator
      json[:relationships] = {
        data: {
          _id: @aggregator.couch_id,
          _type: @aggregator.doctype
        }
      }
    end
    json
  end
end
