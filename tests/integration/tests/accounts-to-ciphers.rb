require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

def setup_ciphers(override_account_attrs = {})
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

  it "links account to cipher and keeps auth attributes" do
    Helpers.scenario "account-to-ciphers"
    Helpers.start_mailhog

    res = setup_ciphers()
    account = res[:account]
    inst = res[:inst]

    bw = res[:bw]
    cipher = bw.items[0]

    db_prefix = inst.db_prefix()
    couch_client = RestClient::Resource.new "http://localhost:5984/"
    res = couch_client["#{URI.escape(db_prefix)}%2Fio-cozy-accounts/#{account.couch_id}"].get()
    account = JSON.parse(res.body)

    # Check that the cipher has been linked
    assert_equal account["relationships"]["vaultCipher"]["data"]["_id"], cipher[:id]

    # Check that the account auth fields have not been removed 
    assert_equal account["auth"]["login"], "Isabelle"
    assert_equal account["auth"]["zipcode"], "64000"

  end
end
