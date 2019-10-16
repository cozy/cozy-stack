require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

def wait_for_file(file)
  10.times do
    sleep 1
    return if File.exist? file
  end
end

describe "An io.cozy.accounts" do
  it "is cleaned via on_delete_account" do
    Helpers.scenario "accounts_cleaning"
    Helpers.start_mailhog

    inst = Instance.create name: "Isabelle"
    Account.create inst, name: "not a bank account"
    source_url = "file://" + File.expand_path("../konnector", __dir__)

    # 1. When an account is deleted, it is cleaned.
    inst.install_konnector "bankone", source_url
    aggregator = Account.create inst, id: ["bank-aggregator", UUID.generate].sample
    accone = Account.create inst, type: "bankone", aggregator: aggregator,
                                  name: Faker::HarryPotter.character
    Trigger.create inst, worker: "konnector", type: "@cron", arguments: "@monthly",
                         message: { konnector: "bankone", account: accone.couch_id }

    job = inst.run_konnector "bankone", accone.couch_id
    done = false
    10.times do
      sleep 1
      done = job.done?(inst)
      break if done
    end
    assert done
    executed = JSON.parse File.read(accone.log)
    assert_equal executed["_id"], accone.couch_id
    File.delete accone.log

    accone.delete inst
    wait_for_file accone.log
    executed = JSON.parse File.read(accone.log)
    assert_equal executed["_id"], aggregator.couch_id
  end
end
