require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

describe "Onboarding a Cozy" do
  it "can be made from the flagship app" do
    Helpers.scenario "onboarding_flagship"
    Helpers.start_mailhog

    inst = Instance.create name: "Jade", locale: "en", onboarded: false
    client = OAuth::Client.create inst
    session_code = client.register_passphrase inst, "cozy"
    page = client.open_authorize_page inst, session_code
    verify_code = client.receive_flagship_code inst
    access_code = client.validate_flagship inst, page, verify_code
    tokens = client.access_token inst, access_code
    refute_nil tokens["access_token"]
    refute_nil tokens["refresh_token"]
    permissions = client.list_permissions inst, tokens
    assert_equal permissions.dig("data", "attributes", "permissions", "rule0", "type"), "*"

    # Check that the passphrase has been correctly set
    inst.open_session

    assert_equal inst.check, []
    inst.remove
  end
end
