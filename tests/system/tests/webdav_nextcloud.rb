require_relative '../boot'
require 'minitest/autorun'
require 'pry-rescue/minitest' unless ENV['CI']


describe "NextCloud" do
  it "can be used with WebDAV" do
    Helpers.scenario "webdav_nextcloud"
    Helpers.start_mailhog

    container = Testcontainers::DockerContainer.new("nextcloud:latest")
    container.add_exposed_ports(80)
    volume = File.expand_path("../nextcloud/before-starting", __dir__)
    container.add_filesystem_bind volume, "/docker-entrypoint-hooks.d/before-starting"
    container.add_env "SQLITE_DATABASE", "nextcloud"
    container.add_env "NEXTCLOUD_ADMIN_USER", "root"
    container.add_env "NEXTCLOUD_ADMIN_PASSWORD", "63a9f0ea7bb98050796b649e85481845"
    container.add_wait_for :logs, /apache2 -D FOREGROUND/

    puts "Start NextCloud container...".green
    container.use do
      host = container.host
      port = container.first_mapped_port
      user = "fred"
      pass = "570a90bfbf8c7eab5dc5d4e26832d5b1"

      inst = Instance.create name: "Fred"
      auth = { login: user, password: pass, url: "http://#{host}:#{port}/" }
      # We need to put the webdav_user_id in the document, as the user_status
      # endpoint from NextCloud doesn't work well in the docker container (it
      # responds with a 404 if the user has logged-in, and sometimes, it even
      # responds with a 500 after that).
      account = Account.create inst, type: "nextcloud",
                                     name: "NextCloud",
                                     auth: auth,
                                     webdav_user_id: "fred"

      nextcloud = Nextcloud.new inst, account.couch_id
      dir_name = "#{Faker::Superhero.name} ⚡️"
      nextcloud.mkdir "/#{dir_name}"

      file_name = "#{Faker::Science.science}.jpg"
      file_path = "../fixtures/wet-cozy_20160910__M4Dz.jpg"
      nextcloud.upload "/#{dir_name}/#{file_name}", file_path
      list = nextcloud.list "/#{dir_name}"
      assert_equal 1, list.dig("meta", "count")
      assert_equal "file", list.dig("data", 0, "attributes", "type")
      assert_equal file_name, list.dig("data", 0, "attributes", "name")
      assert_equal File.size(file_path), list.dig("data", 0, "attributes", "size")
      assert_equal "image/jpeg", list.dig("data", 0, "attributes", "mime")
      assert_equal "image", list.dig("data", 0, "attributes", "class")
    end

    container.remove
  end
end
