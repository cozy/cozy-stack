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
    @failure = opts[:failure]
    @type = opts[:type]
    @auth = opts[:auth]
  end

  def as_json
    json = {
      name: @name,
      log: @log,
      failure: @failure,
      account_type: @type,
      auth: @auth
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
