# Adapted from https://github.com/grosser/maxitest/blob/master/lib/maxitest/timeout.rb
# It looks like it is not compatible with pry-rescue
require 'minitest'
require 'timeout'

module TestTimeout
  class << self
    attr_accessor :timeout
  end

  class TestCaseTimeout < StandardError
    def message
      "Test took too long to finish, aborting."
    end
  end

  def run(*, &block)
    timeout = TestTimeout.timeout || 500
    begin
      ::Timeout.timeout(timeout, TestCaseTimeout) { super }
    rescue TestCaseTimeout => e
      failures << UnexpectedError.new(e)
    end
  end
end

Minitest::Test.send :prepend, TestTimeout
