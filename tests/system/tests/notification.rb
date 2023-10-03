require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

describe "Notification" do
  it "can be sent by the stack" do
    Helpers.scenario "notifications"
    Helpers.start_mailhog

    mail = Faker::Internet.email
    inst = Instance.create email: mail

    at = (Time.now + 5).iso8601
    later = Notification.create inst, at
    created = Notification.create inst

    sleep 2
    received = Email.received kind: "to", query: mail
    assert_equal created.title, received.first.subject

    sleep 4
    received = Email.received kind: "to", query: mail
    assert_equal later.title, received.first.subject
    assert_equal created.title, received.last.subject
  end
end
