require 'minitest'

module CatLogsPlugin
  def before_teardown
    super
    Stack.cat_logs unless passed?
  end
end

class Minitest::Test
  include CatLogsPlugin
end
