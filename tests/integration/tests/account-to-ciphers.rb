require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

def setup_ciphers(override_account_attrs = {})
  puts "1"
  inst = Instance.create name: "Alice"

  bw = Bitwarden.new inst
  bw.login
  assert_equal bw.sync, "Syncing complete."

  source_url = "file://" + File.expand_path("../konnector", __dir__)
  inst.install_konnector "bankone", source_url

  account_attrs = {
    :type => "bankone",
    :name => "Bank one",
    :auth => {:login => "Isabelle", :zipcode => "64000"}
  }

  account_attrs = account_attrs.merge(override_account_attrs)

  account = Account.create(inst, **account_attrs)

  trigger = Trigger.create(
    inst,
    worker: "konnector",
    type: "@cron",
    arguments: "@monthly",
    message: { konnector: "bankone", account: account.couch_id }
  )

  job = inst.run_job "migrations", {:type => "accounts-to-organization"}
  10.times do
    sleep 1
    done = job.done?(inst)
    break if done
  end

  bw.sync

  return {
    :bw => bw,
    :account => account,
    :inst => inst,
    :trigger => trigger
  }
end


describe "Copying accounts to bitwarden ciphers" do
  it "copies accounts to ciphers" do
    Helpers.scenario "account-to-ciphers"
    Helpers.start_mailhog

    res = setup_ciphers()
    bw = res[:bw]

    items = bw.items
    assert_equal items.length, 1

    cipher = items[0]
    assert_equal cipher[:fields].length(), 1
    assert_equal cipher[:fields][0][:name], "zipcode"
    assert_equal cipher[:fields][0][:value], "64000"
  end
end
