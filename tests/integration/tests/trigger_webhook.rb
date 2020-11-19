require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']

describe "A webhook trigger" do
  it "can be used to execute konnectors" do
    Helpers.scenario "webhook_trigger"
    Helpers.start_mailhog

    inst = Instance.create
    source_url = "file://" + File.expand_path("../konnector", __dir__)
    konnector_name = Faker::Cat.name.downcase
    inst.install_konnector konnector_name, source_url
    account = Account.create inst, type: konnector_name, name: Faker::DrWho.character
    args = { "konnector" => konnector_name, "account" => account.couch_id, "foo" => "bar" }
    webhook_url = Job::Webhook.create inst, args

    opts = { :"content-type" => 'application/json' }
    body = { "quote" => Faker::DrWho.quote }
    inst.client[webhook_url].post JSON.generate(body), opts

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

    inst.remove
  end
end
