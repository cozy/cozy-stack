require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

describe "A webhook trigger" do
  it "can be used to execute konnectors" do
    Helpers.scenario "webhook_trigger"
    Helpers.start_mailhog

    # No debounce
    inst = Instance.create
    source_url = "file://" + File.expand_path("../konnector", __dir__)
    konnector_name = Faker::Cat.name.downcase
    inst.install_konnector konnector_name, source_url
    account = Account.create inst, type: konnector_name, name: "1_#{Faker::DrWho.character}"
    args = { "konnector" => konnector_name, "account" => account.couch_id, "foo" => "bar" }
    webhook_url = Trigger::Webhook.create inst, args

    opts = { :"content-type" => 'application/json' }
    body = { "quote" => Faker::DrWho.quote }
    RestClient.post webhook_url, JSON.generate(body), opts

    done = false
    10.times do
      sleep 1
      done = File.exist? account.log
      break if done
    end
    ap konnector_name unless done
    assert done
    executed = JSON.parse File.read(account.log)
    assert_equal executed["account"]["_id"], account.couch_id
    assert_equal executed["fields"], args
    assert_equal executed["payload"], body

    # With debounce
    account2 = Account.create inst, type: konnector_name, name: "2_#{Faker::DrWho.character}"
    args = { "konnector" => konnector_name, "account" => account2.couch_id, "foo" => "bar" }
    webhook_url = Trigger::Webhook.create inst, args, "1s"

    opts = { :"content-type" => 'application/json' }
    RestClient.post webhook_url, JSON.generate(part: 1), opts
    RestClient.post webhook_url, JSON.generate(part: 2), opts
    RestClient.post webhook_url, JSON.generate(part: 3), opts
    RestClient.post webhook_url, JSON.generate(part: 4), opts

    done = false
    10.times do
      sleep 1
      done = File.exist? account2.log
      break if done
    end
    ap konnector_name unless done
    assert done
    executed = JSON.parse File.read(account2.log)
    payloads = [{ "part" => 1 }, { "part" => 2 }, { "part" => 3 }, { "part" => 4 }]
    assert_equal executed["account"]["_id"], account2.couch_id
    assert_equal executed["fields"], args
    assert_equal executed["payload"], { "payloads" => payloads }

    account.delete inst
    account2.delete inst
    inst.remove
  end
end
