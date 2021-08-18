require 'minitest'

module CatLogsPlugin
  def before_teardown
    super
    sleep 1
    Stack.cat_logs unless passed?
  end
end

class Minitest::Test
  include CatLogsPlugin
end
