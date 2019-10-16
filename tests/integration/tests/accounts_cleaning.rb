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

    inst.install_konnector "bankone", source_url
    aggregator = Account.create inst, id: ["bank-aggregator", UUID.generate].sample

    # 1. When an account is deleted, it is cleaned.
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

    # 2. When a konnector is uninstalled, its account is cleaned.
    inst.install_konnector "banktwo", source_url
    acctwo = Account.create inst, type: "banktwo", aggregator: aggregator,
                                  name: Faker::Hobbit.thorins_company
    Trigger.create inst, worker: "konnector", type: "@cron", arguments: "@monthly",
                         message: { konnector: "banktwo", account: acctwo.couch_id }

    assert inst.remove_konnector "banktwo"
    wait_for_file acctwo.log
    executed = JSON.parse File.read(acctwo.log)
    assert_equal executed["_id"], aggregator.couch_id

    # 3. When the instance is deleted, the accounts are cleaned.
    # accthree = Account.create inst, type: "bankone", aggregator: aggregator, name: Faker::Simpsons.character
    # inst.install_konnector "bankthree", source_url
    # other = Account.create inst, id: UUID.generate
    # accfour = Account.create inst, type: "bankthree", aggregator: other, name: Faker::Friends.character
    # assert inst.remove

    # 4. When an instance is going to be deleted but the cleaning fails,
    # the instance is kept and a mail is sent.
  end
end
